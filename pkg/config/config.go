package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds application configuration
type Config struct {
	DatabaseURL   string
	JWTSecret     string
	EncryptionKey string
	Environment   string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() *Config {
	// Load .env file if it exists
	godotenv.Load()

	cfg := &Config{
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/domain_detection?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "your-secret-key-change-me"),
		EncryptionKey: getEnv("ENCRYPTION_KEY", "your-encryption-key-change-me"),
		Environment:   getEnv("ENVIRONMENT", "development"),
	}

	// Log warnings for missing or default secrets in production
	if cfg.Environment == "production" {
		if cfg.JWTSecret == "your-secret-key-change-me" {
			log.Fatal("Production environment detected, but JWT_SECRET not set")
		}
		if cfg.EncryptionKey == "your-encryption-key-change-me" {
			log.Fatal("Production environment detected, but ENCRYPTION_KEY not set")
		}
	}

	return cfg
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
