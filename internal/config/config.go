package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL     string
	JWTSecret       string
	Port            string
	CORSOrigin      string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

func Load() Config {
	accessMinutes := envInt("ACCESS_TOKEN_MINUTES", 15)
	refreshDays := envInt("REFRESH_TOKEN_DAYS", 30)

	return Config{
		DatabaseURL:     env("DATABASE_URL", "postgres://bp:bp@localhost:5432/bloodpressure?sslmode=disable"),
		JWTSecret:       env("JWT_SECRET", "dev-secret-change-in-prod"),
		Port:            env("PORT", "8080"),
		CORSOrigin:      env("CORS_ORIGIN", "*"),
		AccessTokenTTL:  time.Duration(accessMinutes) * time.Minute,
		RefreshTokenTTL: time.Duration(refreshDays) * 24 * time.Hour,
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
