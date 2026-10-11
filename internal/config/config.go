package config

import "os"

// Settings contains runtime settings that are safe to expose to the HTTP layer.
type Settings struct {
	Version       string
	CookieSecure  bool
	SessionTTLMin int
}

func FromEnv() Settings {
	secure := os.Getenv("APP_COOKIE_SECURE") == "true"
	return Settings{
		Version:       envOr("APP_VERSION", "0.1.0"),
		CookieSecure:  secure,
		SessionTTLMin: 60 * 24,
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
