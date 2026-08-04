package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"log/slog"
)

// Config holds all runtime configuration for ForgeTSS.
type Config struct {
	DatabaseURL       string
	ListenAddr        string
	APIKeys           []string
	HorizonEndpoints  []string
	SorobanEndpoints  []string
	MasterSeed        string
	MaxRetryAttempts  int
	RetryBaseDelay    time.Duration
	RetryMultiplier   float64
	QueuePollInterval time.Duration
	RefillBatchSize   int
	LogLevel          slog.Level
}

// Load reads configuration from environment variables and returns a Config.
// It applies sensible defaults for optional fields.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:       getEnvString("DATABASE_URL", "postgres://forgetss:forgetss@localhost:5432/forgetss"),
		ListenAddr:        getEnvString("LISTEN_ADDR", ":8080"),
		HorizonEndpoints:  getEnvStrings("HORIZON_ENDPOINTS", "https://horizon-testnet.stellar.org"),
		SorobanEndpoints:  getEnvStrings("SOROBAN_ENDPOINTS", "https://soroban-testnet.stellar.org"),
		MasterSeed:        os.Getenv("MASTER_SEED"),
		MaxRetryAttempts:  getEnvInt("MAX_RETRY_ATTEMPTS", 5),
		RetryBaseDelay:    getEnvDuration("RETRY_BASE_DELAY", 2*time.Second),
		RetryMultiplier:   getEnvFloat("RETRY_MULTIPLIER", 2.0),
		QueuePollInterval: getEnvDuration("QUEUE_POLL_INTERVAL", 1*time.Second),
		RefillBatchSize:   getEnvInt("REFILL_BATCH_SIZE", 10),
		LogLevel:          getLogLevel("LOG_LEVEL"),
	}

	if keys := os.Getenv("API_KEYS"); keys != "" {
		c.APIKeys = splitCSV(keys)
	}

	if c.MaxRetryAttempts < 0 {
		return nil, fmt.Errorf("MAX_RETRY_ATTEMPTS must be >= 0, got %d", c.MaxRetryAttempts)
	}
	if c.RetryMultiplier < 1.0 {
		return nil, fmt.Errorf("RETRY_MULTIPLIER must be >= 1.0, got %.2f", c.RetryMultiplier)
	}
	if c.RefillBatchSize < 1 {
		return nil, fmt.Errorf("REFILL_BATCH_SIZE must be >= 1, got %d", c.RefillBatchSize)
	}

	return c, nil
}

func getEnvString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvFloat(key string, fallback float64) float64 {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return f
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func getLogLevel(key string) slog.Level {
	s := os.Getenv(key)
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func splitCSV(s string) []string {
	parts := make([]string, 0)
	for _, p := range split(s) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func split(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
