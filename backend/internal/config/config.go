package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env            string
	HTTPPort       int
	GRPCPort       int
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string
	JWTExpiry      time.Duration
	AllowedOrigins []string
	EncryptionKey  string // 64-char hex string (32 bytes) for AES-256-GCM encryption
	LicenseKey string // License key from infrapilot.org
	DataDir    string // Persistent data directory for instance ID etc.
}

func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		Env:            getEnv("ENV", "development"),
		HTTPPort:       getEnvInt("HTTP_PORT", 8080),
		GRPCPort:       getEnvInt("GRPC_PORT", 9090),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://infrapilot:infrapilot@localhost:5432/infrapilot?sslmode=disable"),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		JWTExpiry:      getEnvDuration("JWT_EXPIRY", 24*time.Hour),
		AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		EncryptionKey:  getEnv("ENCRYPTION_KEY", ""),
		LicenseKey:     getEnv("LICENSE_KEY", ""),
		DataDir:        getEnv("DATA_DIR", "/data"),
		// Version is injected at build time via main.version; not read from env here.
	}

	// Validate required fields
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
