package config

import (
	"os"
	"strconv"
)

// Settings contains runtime settings that are safe to expose to the HTTP layer.
type Settings struct {
	Version            string
	CookieSecure       bool
	SessionTTLMin      int
	LoginFailureLimit  int
	CommentLimit       int
}

func FromEnv() Settings {
	secure := os.Getenv("APP_COOKIE_SECURE") == "true"
	ttl := 60 * 24
	if raw := os.Getenv("APP_SESSION_TTL_MIN"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			ttl = parsed
		}
	}
	return Settings{
		Version:       envOr("APP_VERSION", "0.1.0"),
		CookieSecure:  secure,
		SessionTTLMin: ttl,
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
