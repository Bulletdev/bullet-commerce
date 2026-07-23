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
)

const defaultJWTExpiry = 24 * time.Hour

func main() {
	cfg := config.Load()

	setupLogger(cfg.LogLevel)

	dbPool, err := database.NewConnection(cfg.DatabaseURL)
	if err != nil {
		slog.Error("could not connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

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

	hasher := auth.NewArgon2idHasher()

	healthHandler := handlers.NewHealthHandler(dbPool, handlers.HealthInfo{
		PaymentProvider:   cfg.PaymentProvider,
		PaymentConfigured: cfg.ProPayURL != "",
		AIEnabled:         cfg.FeatureAIAssistant && cfg.AnthropicAPIKey != "",
	})
	authHandler := handlers.NewAuthHandler(userRepo, hasher, cfg.JWTSecret, defaultJWTExpiry)
	userHandler := handlers.NewUserHandler(userRepo, addressRepo)
	productHandler := handlers.NewProductHandler(productRepo, variantRepo, mediaRepo, sourceRepo)
	mediaHandler := handlers.NewMediaHandler(mediaRepo, storageProvider)
	categoryHandler := handlers.NewCategoryHandler(categoryRepo)
	cartHandler := handlers.NewCartHandler(cartRepo, productRepo, variantRepo, voucherHandler)
	orderHandler := handlers.NewOrderHandler(orderRepo, cartRepo, addressRepo, paymentRegistry, payment.Name(cfg.PaymentProvider))
	shippingHandler := handlers.NewShippingHandler(shippingProvider)
	searchHandler := handlers.NewSearchHandler(searchService)
	reviewHandler := handlers.NewReviewHandler(reviewRepo)
	webhookHandler := handlers.NewWebhookHandler(orderRepo, paymentRegistry, payment.Name(cfg.PaymentProvider))

	authMiddleware := auth.NewMiddleware(cfg.JWTSecret, userRepo)

	// AI assistant: optional capability, off unless FEATURE_AI_ASSISTANT=true AND a key is set.
	// When inactive the handler is nil and the route is never registered - no key, no endpoint.
	var aiChatHandler *ai.ChatHandler
	aiCfg := ai.Config{Enabled: cfg.FeatureAIAssistant, APIKey: cfg.AnthropicAPIKey, ModelDefault: cfg.AIModelDefault, ModelHard: cfg.AIModelHard}
	if aiCfg.Active() {
		if provider, err := ai.NewClaudeProvider(aiCfg); err != nil {
			slog.Error("AI assistant init failed; endpoint stays disabled", "error", err)
		} else {
			aiChatHandler = ai.NewChatHandler(aiCfg, provider, tools.NewRegistry(searchService, variantRepo, orderRepo))
			slog.Info("AI assistant enabled")
		}
	} else {
		slog.Info("AI assistant disabled (set FEATURE_AI_ASSISTANT=true and ANTHROPIC_API_KEY to enable)")
	}

	r := setupRoutes(cfg, healthHandler, authHandler, userHandler, productHandler, mediaHandler, categoryHandler, cartHandler, orderHandler, shippingHandler, searchHandler, reviewHandler, webhookHandler, aiChatHandler, authMiddleware)

	// Orphaned order cleanup goroutines: two windows, each releases reservations.
	go runCleanup("orphaned pending_payment orders", 30*time.Minute, orderRepo.ExpireOrphanedOrders)
	go runCleanup("unpaid abandoned orders", 15*time.Minute, orderRepo.ExpireUnpaidOrders)

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

func setupRoutes(
	cfg *config.Config,
	hh *handlers.HealthHandler,
	ah *handlers.AuthHandler,
	uh *handlers.UserHandler,
	ph *handlers.ProductHandler,
	mediaH *handlers.MediaHandler,
	ch *handlers.CategoryHandler,
	cartH *handlers.CartHandler,
	oh *handlers.OrderHandler,
	sh *handlers.ShippingHandler,
	searchH *handlers.SearchHandler,
	reviewH *handlers.ReviewHandler,
	wh *handlers.WebhookHandler,
	aiChat *ai.ChatHandler,
	mw *auth.Middleware,
) *mux.Router {
	r := mux.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.BodyLimit(1 << 20)) // 1 MiB

	// Global OPTIONS handler so the CORS middleware intercepts preflight requests
	// before gorilla/mux returns 404 for unmatched methods.
	r.PathPrefix("/").Methods(http.MethodOptions).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		webutils.WriteJSON(w, http.StatusOK, map[string]string{
			"name":    "bullet-commerce",
			"version": "1.0.0",
			"docs":    "https://github.com/Bulletdev/bullet-commerce",
			"health":  "/health",
			"ready":   "/ready",
		})
	}).Methods(http.MethodGet)

	r.HandleFunc("/health", hh.Liveness).Methods(http.MethodGet)
	r.HandleFunc("/ready", hh.Readiness).Methods(http.MethodGet)

	api := r.PathPrefix("/api").Subrouter()

	// Auth endpoints are rate-limited per IP (credential-stuffing / brute-force defense).
	// Its own subrouter so the limiter applies ONLY to register/login, not the whole /api tree.
	authLimited := api.NewRoute().Subrouter()
	authLimited.Use(middleware.RateLimit(10, 10))
	authLimited.HandleFunc("/auth/register", ah.Register).Methods(http.MethodPost)
	authLimited.HandleFunc("/auth/login", ah.Login).Methods(http.MethodPost)

	api.HandleFunc("/products", ph.GetAllProducts).Methods(http.MethodGet)
	api.HandleFunc("/products/search", ph.SearchProducts).Methods(http.MethodGet)
	api.HandleFunc("/products/featured", ph.GetFeaturedProducts).Methods(http.MethodGet)
	api.HandleFunc("/products/category/{id:[0-9a-fA-F-]+}", ph.GetProductsByCategory).Methods(http.MethodGet)
	api.HandleFunc("/products/{id:[0-9a-fA-F-]+}", ph.GetProduct).Methods(http.MethodGet)
	// Public product reviews (approved-only, paginated).
	api.HandleFunc("/products/{id:[0-9a-fA-F-]+}/reviews", reviewH.ListReviews).Methods(http.MethodGet)

	api.HandleFunc("/categories", ch.GetAllCategories).Methods(http.MethodGet)
	api.HandleFunc("/categories/{id:[0-9a-fA-F-]+}", ch.GetCategory).Methods(http.MethodGet)

	api.HandleFunc("/orders/tracking/{number}", oh.TrackOrder).Methods(http.MethodGet)
	api.HandleFunc("/shipping/cep/{cep}", handlers.LookupCep).Methods(http.MethodGet)
	api.HandleFunc("/shipping/calculate", sh.Calculate).Methods(http.MethodPost)

	// Faceted product search (public catalog read).
	api.HandleFunc("/search", searchH.Search).Methods(http.MethodGet)

	// Payment webhook is PUBLIC (no JWT): the PSP proves authenticity by signing the raw
	// body, verified inside the handler. It must stay off the authenticated subrouter.
	api.HandleFunc("/webhooks/payment", wh.HandlePayment).Methods(http.MethodPost)

	protected := api.NewRoute().Subrouter()
	protected.Use(mw.Authenticate)

	protected.HandleFunc("/users/me", uh.GetMe).Methods(http.MethodGet)
	protected.HandleFunc("/users/me", uh.UpdateMe).Methods(http.MethodPut)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses", uh.ListAddresses).Methods(http.MethodGet)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses", uh.AddAddress).Methods(http.MethodPost)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}", uh.UpdateAddress).Methods(http.MethodPut)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}", uh.DeleteAddress).Methods(http.MethodDelete)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}/default", uh.SetDefaultAddress).Methods(http.MethodPatch)
	// Independent billing vs shipping defaults (handlers/model landed in Round A; routes
	// were dropped when setupRoutes was rewritten in Round B - re-registered here).
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}/default/billing", uh.SetDefaultBillingAddress).Methods(http.MethodPatch)
	protected.HandleFunc("/users/{userId:[0-9a-fA-F-]+}/addresses/{addressId:[0-9a-fA-F-]+}/default/shipping", uh.SetDefaultShippingAddress).Methods(http.MethodPatch)

	protected.HandleFunc("/cart", cartH.GetCart).Methods(http.MethodGet)
	protected.HandleFunc("/cart/items", cartH.AddItem).Methods(http.MethodPost)
	protected.HandleFunc("/cart/items/{variantId:[0-9a-fA-F-]+}", cartH.UpdateItem).Methods(http.MethodPut)
	protected.HandleFunc("/cart/items/{variantId:[0-9a-fA-F-]+}", cartH.DeleteItem).Methods(http.MethodDelete)
	protected.HandleFunc("/cart/coupon", cartH.AddCoupon).Methods(http.MethodPost)
	protected.HandleFunc("/cart/coupon/{code}", cartH.RemoveCoupon).Methods(http.MethodDelete)
	protected.HandleFunc("/cart", cartH.ClearCart).Methods(http.MethodDelete)

	// Authenticated product review submission (one per user per product; user_id from JWT).
	protected.HandleFunc("/products/{id:[0-9a-fA-F-]+}/reviews", reviewH.CreateReview).Methods(http.MethodPost)

	// Checkout is authenticated AND rate-limited: order create/pay touch stock and hit the PSP,
	// so they get a tighter per-IP budget than the rest of the authenticated surface. Idempotency
	// (Idempotency-Key header on POST /orders) dedupes double-submits on top of the rate limit.
	ordersLimited := api.NewRoute().Subrouter()
	ordersLimited.Use(mw.Authenticate)
	ordersLimited.Use(middleware.RateLimit(30, 30))
	ordersLimited.HandleFunc("/orders", oh.CreateOrder).Methods(http.MethodPost)
	ordersLimited.HandleFunc("/orders", oh.ListOrders).Methods(http.MethodGet)
	ordersLimited.HandleFunc("/orders/{id:[0-9a-fA-F-]+}", oh.GetOrder).Methods(http.MethodGet)
	ordersLimited.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/cancel", oh.CancelOrder).Methods(http.MethodPatch)
	ordersLimited.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/pay", oh.Pay).Methods(http.MethodPost)

	// Registered only when the AI assistant is active (see main): no key → no route.
	if aiChat != nil {
		protected.Handle("/assistant/chat", aiChat).Methods(http.MethodPost)
	}

	admin := api.NewRoute().Subrouter()
	admin.Use(mw.Authenticate)
	admin.Use(mw.RequireAdmin)

	admin.HandleFunc("/products", ph.CreateProduct).Methods(http.MethodPost)
	admin.HandleFunc("/products/{id:[0-9a-fA-F-]+}", ph.UpdateProduct).Methods(http.MethodPut)
	admin.HandleFunc("/products/{id:[0-9a-fA-F-]+}", ph.DeleteProduct).Methods(http.MethodDelete)
	admin.HandleFunc("/products/{id:[0-9a-fA-F-]+}/stock", ph.UpdateStock).Methods(http.MethodPatch)
	admin.HandleFunc("/products/{id:[0-9a-fA-F-]+}/variants", ph.CreateVariant).Methods(http.MethodPost)
	admin.HandleFunc("/products/{id:[0-9a-fA-F-]+}/variants/{variantId:[0-9a-fA-F-]+}/stock", ph.UpdateVariantStock).Methods(http.MethodPatch)

	// Product media: register by URL, mint a presigned upload URL (501 when storage is off),
	// and delete. All admin-only.
	admin.HandleFunc("/products/{id:[0-9a-fA-F-]+}/media", mediaH.AddMedia).Methods(http.MethodPost)
	admin.HandleFunc("/media/upload-url", mediaH.UploadURL).Methods(http.MethodPost)
	admin.HandleFunc("/media/{id:[0-9a-fA-F-]+}", mediaH.DeleteMedia).Methods(http.MethodDelete)

	admin.HandleFunc("/categories", ch.CreateCategory).Methods(http.MethodPost)
	admin.HandleFunc("/categories/{id:[0-9a-fA-F-]+}", ch.UpdateCategory).Methods(http.MethodPut)
	admin.HandleFunc("/categories/{id:[0-9a-fA-F-]+}", ch.DeleteCategory).Methods(http.MethodDelete)

	admin.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/tracking", oh.UpdateTracking).Methods(http.MethodPatch)
	// Refund (financial reversal + opt-in per-item restock). Admin-only; 501 if the PSP can't refund.
	admin.HandleFunc("/orders/{id:[0-9a-fA-F-]+}/refund", oh.RefundOrder).Methods(http.MethodPost)

	// Review moderation (approve/reject) - recomputes the product's rating aggregate.
	admin.HandleFunc("/reviews/{id:[0-9a-fA-F-]+}/moderate", reviewH.ModerateReview).Methods(http.MethodPatch)

	return r
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
