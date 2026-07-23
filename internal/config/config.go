package config

import (
	"bullet-commerce/internal/models"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL    string
	JWTSecret      string
	Port           string
	AllowedOrigins string
	LogLevel       string
	CacheEnabled   bool
	CacheL1MaxSize int
	CacheTTL       time.Duration

	// DefaultCurrency is the app-wide currency. It is sourced from models.DefaultCurrency,
	// which stays the SINGLE source of truth (also the DEFAULT in migration 000009); this
	// field just exposes it to code that reads config without importing models.
	DefaultCurrency string

	// Payment (12-factor: provider selected by env, secrets never in code).
	PaymentProvider  string
	ProPayURL        string
	GoToProPaySecret string
	ProPayToGoSecret string
	ProPayTimeout    time.Duration

	// Shipping.
	ShippingProvider  string
	ShippingSenderCEP string

	// Object storage (12-factor: optional, gated by STORAGE_ENABLED; secrets never in code).
	// When disabled the media vertical still serves the URL-reference flow - only presigned
	// uploads are unavailable.
	StorageEnabled       bool
	StorageBucket        string
	StorageEndpoint      string
	StorageRegion        string
	StorageAccessKey     string
	StorageSecret        string
	StoragePublicBaseURL string

	// AI assistant (Phase: wiring only). FeatureAIAssistant gates the feature; the key is
	// NOT validated at startup so the service boots without it when the feature is off.
	FeatureAIAssistant bool
	AnthropicAPIKey    string
	AIModelDefault     string
	AIModelHard        string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, using environment variables")
	}

	dbURL := getEnv("DATABASE_URL", "")
	if dbURL == "" {
		slog.Error("DATABASE_URL environment variable not set")
		os.Exit(1)
	}

	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" {
		slog.Error("JWT_SECRET environment variable not set")
		os.Exit(1)
	}

	return &Config{
		DatabaseURL:    dbURL,
		JWTSecret:      jwtSecret,
		Port:           getEnv("PORT", "4444"),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:8880,http://localhost:5173"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		CacheEnabled:   getBool("CACHE_ENABLED", true),
		CacheL1MaxSize: getInt("CACHE_L1_MAX_SIZE", 1000),
		CacheTTL:       getDuration("CACHE_TTL_PRODUCTS", 5*time.Minute),

		// models.DefaultCurrency is the single source of truth; config only re-exposes it.
		DefaultCurrency: models.DefaultCurrency,

		PaymentProvider:  getEnv("PAYMENT_PROVIDER", "propay"),
		ProPayURL:        getEnv("PROPAY_URL", ""),
		GoToProPaySecret: getEnv("GO_TO_PROPAY_SECRET", ""),
		ProPayToGoSecret: getEnv("PROPAY_TO_GO_SECRET", ""),
		ProPayTimeout:    getDuration("PROPAY_TIMEOUT", 5*time.Second),

		ShippingProvider:  getEnv("SHIPPING_PROVIDER", "table"),
		ShippingSenderCEP: getEnv("SHIPPING_SENDER_CEP", ""),

		StorageEnabled:       getBool("STORAGE_ENABLED", false),
		StorageBucket:        getEnv("STORAGE_BUCKET", ""),
		StorageEndpoint:      getEnv("STORAGE_ENDPOINT", ""),
		StorageRegion:        getEnv("STORAGE_REGION", ""),
		StorageAccessKey:     getEnv("STORAGE_ACCESS_KEY", ""),
		StorageSecret:        getEnv("STORAGE_SECRET", ""),
		StoragePublicBaseURL: getEnv("STORAGE_PUBLIC_BASE_URL", ""),

		FeatureAIAssistant: getBool("FEATURE_AI_ASSISTANT", false),
		AnthropicAPIKey:    getEnv("ANTHROPIC_API_KEY", ""),
		AIModelDefault:     getEnv("AI_MODEL_DEFAULT", "claude-haiku-4-5"),
		AIModelHard:        getEnv("AI_MODEL_HARD", "claude-sonnet-5"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
