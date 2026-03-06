package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config holds all runtime settings for the application.
type Config struct {
	Addr                string
	BaseURL             string
	DBDriver            string
	DBPath              string
	DBDSN               string
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRedirectURL  string
	SessionSecret       string
	AllowedOrigins      []string
	ForceSecureCookies  bool
	OpenAIAPIKey        string
	OpenAIModel         string
}

// Load reads environment variables and applies sensible defaults.
func Load() Config {
	baseURL := getEnv("RHYTHMIQ_BASE_URL", "http://127.0.0.1:8080")
	forceSecureCookies := isHTTPSURL(baseURL)

	return Config{
		Addr:                getEnv("RHYTHMIQ_ADDR", ":8080"),
		BaseURL:             baseURL,
		DBDriver:            getEnv("RHYTHMIQ_DB_DRIVER", "postgres"),
		DBPath:              getEnv("RHYTHMIQ_DB_PATH", "./rhythmiq.db"),
		DBDSN:               getEnv("RHYTHMIQ_DB_DSN", "postgres://rhythmiq:rhythmiq@127.0.0.1:5432/rhythmiq?sslmode=disable"),
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		SpotifyRedirectURL:  getEnv("SPOTIFY_REDIRECT_URL", fmt.Sprintf("%s/api/auth/callback", baseURL)),
		SessionSecret:       getEnv("RHYTHMIQ_SESSION_SECRET", "rhythmiq-dev-session-secret-change-me"),
		AllowedOrigins:      resolveAllowedOrigins(baseURL, os.Getenv("RHYTHMIQ_ALLOWED_ORIGINS")),
		ForceSecureCookies:  forceSecureCookies,
		OpenAIAPIKey:        os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:         getEnv("OPENAI_MODEL", "gpt-4.1-mini"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func resolveAllowedOrigins(baseURL, raw string) []string {
	if strings.TrimSpace(raw) != "" {
		return splitCSV(raw)
	}

	// Development defaults: allow local Vite origins when serving from loopback.
	if isLoopbackURL(baseURL) {
		return []string{
			"http://localhost:5173",
			"http://127.0.0.1:5173",
			baseURL,
		}
	}

	// Production default: only the configured public origin.
	return []string{baseURL}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isHTTPSURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https")
}

func isLoopbackURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "127.0.0.1" || host == "::1" || host == "localhost"
}
