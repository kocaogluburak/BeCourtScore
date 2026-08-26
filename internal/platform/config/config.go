package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       []byte
	GoogleClientIDs []string
	AppleClientIDs  []string
	CORSOrigins     []string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     mustEnv("DATABASE_URL"),
		JWTSecret:       []byte(mustEnv("JWT_SECRET")),
		GoogleClientIDs: splitEnv("GOOGLE_CLIENT_IDS", ","),
		AppleClientIDs:  splitEnv("APPLE_CLIENT_IDS", ","),
		CORSOrigins:     splitEnvDefault("CORS_ORIGINS", ",", []string{"*"}),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("required env var missing: " + key)
	}
	return v
}

func splitEnv(key, sep string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(v, sep) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitEnvDefault(key, sep string, def []string) []string {
	result := splitEnv(key, sep)
	if len(result) == 0 {
		return def
	}
	return result
}
