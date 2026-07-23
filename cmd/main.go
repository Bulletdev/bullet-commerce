package main

import (
	"bullet-commerce/internal/addresses"
	"bullet-commerce/internal/ai"
	"bullet-commerce/internal/ai/tools"
	"bullet-commerce/internal/auth"
	"bullet-commerce/internal/cart"
	"bullet-commerce/internal/categories"
	"bullet-commerce/internal/charges"
	"bullet-commerce/internal/config"
	"bullet-commerce/internal/coupons"
	"bullet-commerce/internal/database"
	"bullet-commerce/internal/events"
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/media"
	"bullet-commerce/internal/middleware"
	"bullet-commerce/internal/orders"
	"bullet-commerce/internal/payment"
	"bullet-commerce/internal/payment/propay"
	"bullet-commerce/internal/products"
	"bullet-commerce/internal/promotions"
	"bullet-commerce/internal/reviews"
	"bullet-commerce/internal/search"
	"bullet-commerce/internal/shipping"
	"bullet-commerce/internal/sourcing"
	"bullet-commerce/internal/storage"
	"bullet-commerce/internal/users"
	"bullet-commerce/internal/variants"
	"bullet-commerce/internal/webutils"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultJWTExpiry = 24 * time.Hour

// Shared route templates: the UUID-constrained path segments repeat across the public,
// protected and admin subrouters, so they live here once to keep the strings identical.
const (
	productByIDPath  = "/products/{id:[0-9a-fA-F-]+}"
	categoryByIDPath = "/categories/{id:[0-9a-fA-F-]+}"
	variantIDPath    = "{variantId:[0-9a-fA-F-]+}"
)

// repositories groups the data-access ports plus the shared pricing helper so main can
// wire handlers without threading a dozen separate values through the call graph.
type repositories struct {
	user     users.UserRepository
	product  products.ProductRepository
	variant  variants.VariantRepository
	media    media.MediaRepository
	category categories.CategoryRepository
	address  addresses.AddressRepository
	cart     cart.CartRepository
	review   reviews.ReviewRepository
	search   search.Service
	source   sourcing.SourceRepository
	order    orders.OrderRepository
	voucher  *promotions.CouponHandler
}

// providers groups the outbound infrastructure adapters built from config.
type providers struct {
	shipping shipping.Provider
	payment  *payment.Registry
	storage  storage.Provider
}

// httpHandlers bundles every HTTP handler so route registration takes one value
// instead of one argument per domain.
type httpHandlers struct {
	health   *handlers.HealthHandler
	auth     *handlers.AuthHandler
	user     *handlers.UserHandler
	product  *handlers.ProductHandler
	media    *handlers.MediaHandler
	category *handlers.CategoryHandler
	cart     *handlers.CartHandler
	order    *handlers.OrderHandler
	shipping *handlers.ShippingHandler
	search   *handlers.SearchHandler
	review   *handlers.ReviewHandler
	webhook  *handlers.WebhookHandler
	ai       *ai.ChatHandler
}

func main() {
	cfg := config.Load()

	setupLogger(cfg.LogLevel)

	dbPool, err := database.NewConnection(cfg.DatabaseURL)
	if err != nil {
		slog.Error("could not connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	repos := buildRepositories(dbPool)
	provs := buildProviders(cfg)
	h := buildHandlers(cfg, dbPool, repos, provs)

	authMiddleware := auth.NewMiddleware(cfg.JWTSecret, repos.user)

	r := setupRoutes(cfg, h, authMiddleware)

	startCleanupWorkers(repos.order)

	runServer(r, cfg)
}

func buildRepositories(dbPool *pgxpool.Pool) repositories {
	userRepo := users.NewPostgresUserRepository(dbPool)
	productRepo := products.NewPostgresProductRepository(dbPool)
	variantRepo := variants.NewPostgresVariantRepository(dbPool)
	mediaRepo := media.NewPostgresMediaRepository(dbPool)
	categoryRepo := categories.NewPostgresCategoryRepository(dbPool)
	addressRepo := addresses.NewPostgresAddressRepository(dbPool)
	cartRepo := cart.NewPostgresCartRepository(dbPool)
	chargeRepo := charges.NewPostgresChargeRepository(dbPool)
	couponRepo := coupons.NewPostgresCouponRepository(dbPool)
	reviewRepo := reviews.NewPostgresReviewRepository(dbPool)
	searchService := search.NewPostgresService(dbPool)

	// Real promotion pricing: the coupon handler validates + prices codes against the
	// coupons table, replacing the no-op. Shared by the cart (live re-pricing) and the
	// order repo (freeze at checkout).
	voucherHandler := promotions.NewCouponHandler(couponRepo)

	// In-process event bus: order.placed / payment.confirmed are published by the order
	// repo AFTER the owning transaction commits (handlers see only durable facts).
	bus := events.NewInProcessBus()

	// Sourcing: resolve the default stock location and build the SingleSourceAllocator that
	// routes every order line to it (transparent V1 - Scale swaps in a multi-source allocator
	// behind the same port). A missing default source means the 000020 seed never ran.
	sourceRepo := sourcing.NewPostgresSourceRepository(dbPool)
	defaultSource, err := sourceRepo.GetDefault(context.Background())
	if err != nil {
		slog.Error("could not resolve default stock source", "error", err)
		os.Exit(1)
	}
	allocator := sourcing.NewSingleSourceAllocator(defaultSource.ID)

	orderRepo := orders.NewPostgresOrderRepository(dbPool, variantRepo, chargeRepo, bus, voucherHandler, couponRepo, allocator)

	return repositories{
		user:     userRepo,
		product:  productRepo,
		variant:  variantRepo,
		media:    mediaRepo,
		category: categoryRepo,
		address:  addressRepo,
		cart:     cartRepo,
		review:   reviewRepo,
		search:   searchService,
		source:   sourceRepo,
		order:    orderRepo,
		voucher:  voucherHandler,
	}
}

func buildProviders(cfg *config.Config) providers {
	// Shipping provider (12-factor: origin CEP and rules from config).
	shippingProvider := shipping.NewTableProvider(cfg.ShippingSenderCEP, shipping.DefaultBrazilRules())

	// Payment registry: register configured PSPs and verify the selected one exists.
	paymentRegistry := payment.NewRegistry()
	if cfg.ProPayURL != "" {
		paymentRegistry.Register(propay.New(propay.Config{
			URL:        cfg.ProPayURL,
			GoToSecret: cfg.GoToProPaySecret,
			ToGoSecret: cfg.ProPayToGoSecret,
			Timeout:    cfg.ProPayTimeout,
		}))
	}
	if _, err := paymentRegistry.Get(payment.Name(cfg.PaymentProvider)); err != nil {
		slog.Error("configured payment provider not available", "provider", cfg.PaymentProvider, "error", err)
	}

	// Object storage (12-factor, gated). A disabled or half-built config yields a nil provider:
	// the media upload endpoint then answers 501 while the URL-reference media flow keeps working.
	var storageProvider storage.Provider
	if provider, err := storage.NewS3Provider(storage.Config{
		Enabled:       cfg.StorageEnabled,
		Bucket:        cfg.StorageBucket,
		Endpoint:      cfg.StorageEndpoint,
		Region:        cfg.StorageRegion,
		AccessKey:     cfg.StorageAccessKey,
		Secret:        cfg.StorageSecret,
		PublicBaseURL: cfg.StoragePublicBaseURL,
	}); err != nil {
		if errors.Is(err, storage.ErrStorageDisabled) {
			slog.Info("storage disabled: URL-only media")
		} else {
			slog.Error("storage init failed; URL-only media", "error", err)
		}
	} else {
		storageProvider = provider
	}

	return providers{
		shipping: shippingProvider,
		payment:  paymentRegistry,
		storage:  storageProvider,
	}
}

func buildHandlers(cfg *config.Config, dbPool *pgxpool.Pool, repos repositories, provs providers) httpHandlers {
	hasher := auth.NewArgon2idHasher()

	return httpHandlers{
		health: handlers.NewHealthHandler(dbPool, handlers.HealthInfo{
			PaymentProvider:   cfg.PaymentProvider,
			PaymentConfigured: cfg.ProPayURL != "",
			AIEnabled:         cfg.FeatureAIAssistant && cfg.AnthropicAPIKey != "",
		}),
		auth:     handlers.NewAuthHandler(repos.user, hasher, cfg.JWTSecret, defaultJWTExpiry),
		user:     handlers.NewUserHandler(repos.user, repos.address),
		product:  handlers.NewProductHandler(repos.product, repos.variant, repos.media, repos.source),
		media:    handlers.NewMediaHandler(repos.media, provs.storage),
		category: handlers.NewCategoryHandler(repos.category),
		cart:     handlers.NewCartHandler(repos.cart, repos.product, repos.variant, repos.voucher),
		order:    handlers.NewOrderHandler(repos.order, repos.cart, repos.address, provs.payment, payment.Name(cfg.PaymentProvider)),
		shipping: handlers.NewShippingHandler(provs.shipping),
		search:   handlers.NewSearchHandler(repos.search),
		review:   handlers.NewReviewHandler(repos.review),
		webhook:  handlers.NewWebhookHandler(repos.order, provs.payment, payment.Name(cfg.PaymentProvider)),
		ai:       buildAIHandler(cfg, repos),
	}
}

// buildAIHandler returns the assistant chat handler, or nil when the AI assistant is
// inactive (off unless FEATURE_AI_ASSISTANT=true AND a key is set). Nil means the route
// is never registered - no key, no endpoint.
func buildAIHandler(cfg *config.Config, repos repositories) *ai.ChatHandler {
	aiCfg := ai.Config{Enabled: cfg.FeatureAIAssistant, APIKey: cfg.AnthropicAPIKey, ModelDefault: cfg.AIModelDefault, ModelHard: cfg.AIModelHard}
	if !aiCfg.Active() {
		slog.Info("AI assistant disabled (set FEATURE_AI_ASSISTANT=true and ANTHROPIC_API_KEY to enable)")
		return nil
	}

	provider, err := ai.NewClaudeProvider(aiCfg)
	if err != nil {
		slog.Error("AI assistant init failed; endpoint stays disabled", "error", err)
		return nil
	}

	slog.Info("AI assistant enabled")
	return ai.NewChatHandler(aiCfg, provider, tools.NewRegistry(repos.search, repos.variant, repos.order))
}

// startCleanupWorkers launches the orphaned-order cleanup goroutines: two windows, each
// releases reservations.
func startCleanupWorkers(orderRepo orders.OrderRepository) {
	go runCleanup("orphaned pending_payment orders", 30*time.Minute, orderRepo.ExpireOrphanedOrders)
	go runCleanup("unpaid abandoned orders", 15*time.Minute, orderRepo.ExpireUnpaidOrders)
}

func runServer(r *mux.Router, cfg *config.Config) {
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func setupRoutes(cfg *config.Config, h httpHandlers, mw *auth.Middleware) *mux.Router {
	r := mux.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.BodyLimit(1 << 20)) // 1 MiB

	// Global OPTIONS handler so the CORS middleware intercepts preflight requests
	// before gorilla/mux returns 404 for unmatched methods.
	r.PathPrefix("/").Methods(http.MethodOptions).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally empty: the CORS middleware answers preflight; this route only exists
		// so gorilla/mux matches OPTIONS instead of returning 404.
	})

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		webutils.WriteJSON(w, http.StatusOK, map[string]string{
			"name":    "bullet-commerce",
			"version": "1.0.0",
			"docs":    "https://github.com/Bulletdev/bullet-commerce",
			"health":  "/health",
			"ready":   "/ready",
		})
	}).Methods(http.MethodGet)

	r.HandleFunc("/health", h.health.Liveness).Methods(http.MethodGet)
	r.HandleFunc("/ready", h.health.Readiness).Methods(http.MethodGet)

	api := r.PathPrefix("/api").Subrouter()

	registerAuthLimitedRoutes(api, h)
	registerPublicRoutes(api, h)
	registerProtectedRoutes(api, h, mw)
	registerOrderRoutes(api, h, mw)
	registerAdminRoutes(api, h, mw)

	return r
}

func registerAuthLimitedRoutes(api *mux.Router, h httpHandlers) {
	// Auth endpoints are rate-limited per IP (credential-stuffing / brute-force defense).
	// Its own subrouter so the limiter applies ONLY to register/login, not the whole /api tree.
	authLimited := api.NewRoute().Subrouter()
	authLimited.Use(middleware.RateLimit(10, 10))
	authLimited.HandleFunc("/auth/register", h.auth.Register).Methods(http.MethodPost)
	authLimited.HandleFunc("/auth/login", h.auth.Login).Methods(http.MethodPost)
}

func registerPublicRoutes(api *mux.Router, h httpHandlers) {
	api.HandleFunc("/products", h.product.GetAllProducts).Methods(http.MethodGet)
	api.HandleFunc("/products/search", h.product.SearchProducts).Methods(http.MethodGet)
	api.HandleFunc("/products/featured", h.product.GetFeaturedProducts).Methods(http.MethodGet)
	api.HandleFunc("/products/category/{id:[0-9a-fA-F-]+}", h.product.GetProductsByCategory).Methods(http.MethodGet)
	api.HandleFunc(productByIDPath, h.product.GetProduct).Methods(http.MethodGet)
	// Public product reviews (approved-only, paginated).
	api.HandleFunc(productByIDPath+"/reviews", h.review.ListReviews).Methods(http.MethodGet)

	api.HandleFunc("/categories", h.category.GetAllCategories).Methods(http.MethodGet)
	api.HandleFunc(categoryByIDPath, h.category.GetCategory).Methods(http.MethodGet)

	api.HandleFunc("/orders/tracking/{number}", h.order.TrackOrder).Methods(http.MethodGet)
	api.HandleFunc("/shipping/cep/{cep}", handlers.LookupCep).Methods(http.MethodGet)
	api.HandleFunc("/shipping/calculate", h.shipping.Calculate).Methods(http.MethodPost)

	// Faceted product search (public catalog read).
	api.HandleFunc("/search", h.search.Search).Methods(http.MethodGet)

	// Payment webhook is PUBLIC (no JWT): the PSP proves authenticity by signing the raw
	// body, verified inside the handler. It must stay off the authenticated subrouter.
	api.HandleFunc("/webhooks/payment", h.webhook.HandlePayment).Methods(http.MethodPost)
}

func registerProtectedRoutes(api *mux.Router, h httpHandlers, mw *auth.Middleware) {
	protected := api.NewRoute().Subrouter()
	protected.Use(mw.Authenticate)

	protected.HandleFunc("/users/me", h.user.GetMe).Methods(http.MethodGet)
	protected.HandleFunc("/users/me", h.user.UpdateMe).Methods(http.MethodPut)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses", h.user.ListAddresses).Methods(http.MethodGet)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses", h.user.AddAddress).Methods(http.MethodPost)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}", h.user.UpdateAddress).Methods(http.MethodPut)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}", h.user.DeleteAddress).Methods(http.MethodDelete)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}/default", h.user.SetDefaultAddress).Methods(http.MethodPatch)
	// Independent billing vs shipping defaults (handlers/model landed in Round A; routes
	// were dropped when setupRoutes was rewritten in Round B - re-registered here).
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}/default/billing", h.user.SetDefaultBillingAddress).Methods(http.MethodPatch)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}/default/shipping", h.user.SetDefaultShippingAddress).Methods(http.MethodPatch)

	protected.HandleFunc("/cart", h.cart.GetCart).Methods(http.MethodGet)
	protected.HandleFunc("/cart/items", h.cart.AddItem).Methods(http.MethodPost)
	protected.HandleFunc("/cart/items/{variantId:[0-9a-fA-F-]+}", h.cart.UpdateItem).Methods(http.MethodPut)
	protected.HandleFunc("/cart/items/{variantId:[0-9a-fA-F-]+}", h.cart.DeleteItem).Methods(http.MethodDelete)
	protected.HandleFunc("/cart/coupon", h.cart.AddCoupon).Methods(http.MethodPost)
	protected.HandleFunc("/cart/coupon/{code}", h.cart.RemoveCoupon).Methods(http.MethodDelete)
	protected.HandleFunc("/cart", h.cart.ClearCart).Methods(http.MethodDelete)

	// Authenticated product review submission (one per user per product; user_id from JWT).
	protected.HandleFunc(productByIDPath+"/reviews", h.review.CreateReview).Methods(http.MethodPost)

	// Registered only when the AI assistant is active (see buildAIHandler): no key, no route.
	if h.ai != nil {
		protected.Handle("/assistant/chat", h.ai).Methods(http.MethodPost)
	}
}

func registerOrderRoutes(api *mux.Router, h httpHandlers, mw *auth.Middleware) {
	// Checkout is authenticated AND rate-limited: order create/pay touch stock and hit the PSP,
	// so they get a tighter per-IP budget than the rest of the authenticated surface. Idempotency
	// (Idempotency-Key header on POST /orders) dedupes double-submits on top of the rate limit.
	ordersLimited := api.NewRoute().Subrouter()
	ordersLimited.Use(mw.Authenticate)
	ordersLimited.Use(middleware.RateLimit(30, 30))
	ordersLimited.HandleFunc("/orders", h.order.CreateOrder).Methods(http.MethodPost)
	ordersLimited.HandleFunc("/orders", h.order.ListOrders).Methods(http.MethodGet)
	ordersLimited.HandleFunc("/orders/{id:[0-9a-fA-F-]+}", h.order.GetOrder).Methods(http.MethodGet)
	ordersLimited.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/cancel", h.order.CancelOrder).Methods(http.MethodPatch)
	ordersLimited.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/pay", h.order.Pay).Methods(http.MethodPost)
}

func registerAdminRoutes(api *mux.Router, h httpHandlers, mw *auth.Middleware) {
	admin := api.NewRoute().Subrouter()
	admin.Use(mw.Authenticate)
	admin.Use(mw.RequireAdmin)

	admin.HandleFunc("/products", h.product.CreateProduct).Methods(http.MethodPost)
	admin.HandleFunc(productByIDPath, h.product.UpdateProduct).Methods(http.MethodPut)
	admin.HandleFunc(productByIDPath, h.product.DeleteProduct).Methods(http.MethodDelete)
	admin.HandleFunc(productByIDPath+"/stock", h.product.UpdateStock).Methods(http.MethodPatch)
	admin.HandleFunc(productByIDPath+"/variants", h.product.CreateVariant).Methods(http.MethodPost)
	admin.HandleFunc(productByIDPath+"/variants/"+variantIDPath+"/stock", h.product.UpdateVariantStock).Methods(http.MethodPatch)

	// Product media: register by URL, mint a presigned upload URL (501 when storage is off),
	// and delete. All admin-only.
	admin.HandleFunc(productByIDPath+"/media", h.media.AddMedia).Methods(http.MethodPost)
	admin.HandleFunc("/media/upload-url", h.media.UploadURL).Methods(http.MethodPost)
	admin.HandleFunc("/media/{id:[0-9a-fA-F-]+}", h.media.DeleteMedia).Methods(http.MethodDelete)

	admin.HandleFunc("/categories", h.category.CreateCategory).Methods(http.MethodPost)
	admin.HandleFunc(categoryByIDPath, h.category.UpdateCategory).Methods(http.MethodPut)
	admin.HandleFunc(categoryByIDPath, h.category.DeleteCategory).Methods(http.MethodDelete)

	admin.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/tracking", h.order.UpdateTracking).Methods(http.MethodPatch)
	// Refund (financial reversal + opt-in per-item restock). Admin-only; 501 if the PSP can't refund.
	admin.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/refund", h.order.RefundOrder).Methods(http.MethodPost)

	// Review moderation (approve/reject) - recomputes the product's rating aggregate.
	admin.HandleFunc("/reviews/{id:[0-9a-fA-F-]+}/moderate", h.review.ModerateReview).Methods(http.MethodPatch)
}

// runCleanup runs a periodic expiry pass. WHY generic: both cleanup windows
// (pending_payment > 30min and unpaid > 15min) share the exact same loop, differing only
// in the repository method they call.
func runCleanup(name string, interval time.Duration, expire func(context.Context) (int64, error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		n, err := expire(ctx)
		cancel()
		if err != nil {
			slog.Error("order cleanup failed", "job", name, "error", err)
		} else if n > 0 {
			slog.Info("expired orders", "job", name, "count", n)
		}
	}
}

func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}
