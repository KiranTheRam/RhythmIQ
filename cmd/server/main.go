package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rhythmiq/internal/api"
	"rhythmiq/internal/config"
	"rhythmiq/internal/db"
	"rhythmiq/internal/service"
	"rhythmiq/internal/spotify"
)

func main() {
	cfg := config.Load()
	if err := validateSecurityConfig(cfg); err != nil {
		log.Fatalf("invalid security configuration: %v", err)
	}

	repo, err := db.New(cfg.DBDriver, cfg.DBPath, cfg.DBDSN, cfg.SessionSecret)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer repo.Close()

	spotifyClient := spotify.NewClient(cfg)
	metrics := service.NewMetricsService(repo, spotifyClient)
	recommender := service.NewRecommendationService(cfg)

	apiServer := api.NewServer(cfg, repo, spotifyClient, metrics, recommender)
	go startAutoSnapshotCollector(repo, metrics)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", apiServer.Routes()))

	staticHandler := buildStaticHandler("web/dist")
	mux.Handle("/", staticHandler)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           loggingMiddleware(globalSecurityHeadersMiddleware(cfg.ForceSecureCookies, mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	log.Printf("RhythmIQ server listening on %s", cfg.Addr)
	log.Printf("Database driver: %s", cfg.DBDriver)
	if spotifyClient.IsConfigured() {
		log.Printf("Spotify OAuth redirect URI: %s", cfg.SpotifyRedirectURL)
		if warning := spotifyRedirectWarning(cfg.SpotifyRedirectURL); warning != "" {
			log.Printf("Spotify config warning: %s", warning)
		}
	} else {
		log.Printf("Spotify is not configured yet. Set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET.")
	}

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}

func buildStaticHandler(distDir string) http.Handler {
	abs, err := filepath.Abs(distDir)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "failed to resolve dist path", http.StatusInternalServerError)
		})
	}
	indexPath := filepath.Join(abs, "index.html")

	if _, err := os.Stat(indexPath); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintln(w, "RhythmIQ API is running. Build frontend with: cd web && npm install && npm run build")
		})
	}

	fs := http.FileServer(http.Dir(abs))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		cleanPath := filepath.Clean(r.URL.Path)
		relPath := strings.TrimPrefix(cleanPath, string(os.PathSeparator))
		candidate := filepath.Join(abs, relPath)
		if strings.HasSuffix(r.URL.Path, "/") {
			candidate = filepath.Join(candidate, "index.html")
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}

		http.ServeFile(w, r, indexPath)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func globalSecurityHeadersMiddleware(forceHSTS bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if forceHSTS && isSecureRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func isSecureRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func spotifyRedirectWarning(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "SPOTIFY_REDIRECT_URL is not a valid URL."
	}

	host := strings.ToLower(parsed.Hostname())

	if host == "google.com" || host == "www.google.com" {
		return "SPOTIFY_REDIRECT_URL points to google.com. It must point back to your RhythmIQ callback endpoint."
	}

	if host == "localhost" {
		return "Spotify no longer accepts localhost redirect URIs. Use http://127.0.0.1:8080/api/auth/callback and register it in your Spotify app."
	}

	if parsed.Scheme == "http" && host != "127.0.0.1" {
		return "Non-HTTPS redirect URIs are only safe for loopback testing. Use https in production and 127.0.0.1 for local testing."
	}

	if !strings.HasSuffix(parsed.Path, "/api/auth/callback") {
		return "SPOTIFY_REDIRECT_URL should normally end with /api/auth/callback and match your Spotify app settings exactly."
	}

	return ""
}

func validateSecurityConfig(cfg config.Config) error {
	baseParsed, err := url.Parse(cfg.BaseURL)
	if err != nil || baseParsed.Scheme == "" || baseParsed.Host == "" {
		return fmt.Errorf("RHYTHMIQ_BASE_URL must be a valid absolute URL")
	}

	baseHost := strings.ToLower(baseParsed.Hostname())
	loopbackBase := baseHost == "127.0.0.1" || baseHost == "::1" || baseHost == "localhost"

	if !loopbackBase {
		if !strings.EqualFold(baseParsed.Scheme, "https") {
			return fmt.Errorf("RHYTHMIQ_BASE_URL must use https for public deployments")
		}
		if len(cfg.AllowedOrigins) == 0 {
			return fmt.Errorf("at least one allowed origin is required for public deployments")
		}
		if cfg.SessionSecret == "rhythmiq-dev-session-secret-change-me" || len(cfg.SessionSecret) < 32 {
			return fmt.Errorf("RHYTHMIQ_SESSION_SECRET must be at least 32 characters and not use the development default")
		}
		redirectParsed, err := url.Parse(cfg.SpotifyRedirectURL)
		if err != nil || redirectParsed.Scheme == "" || redirectParsed.Host == "" {
			return fmt.Errorf("SPOTIFY_REDIRECT_URL must be a valid absolute URL")
		}
		if !strings.EqualFold(redirectParsed.Scheme, "https") {
			return fmt.Errorf("SPOTIFY_REDIRECT_URL must use https for public deployments")
		}
		for _, origin := range cfg.AllowedOrigins {
			if strings.Contains(origin, "*") {
				return fmt.Errorf("RHYTHMIQ_ALLOWED_ORIGINS must not contain wildcard origins for public deployments")
			}

			parsedOrigin, err := url.Parse(origin)
			if err != nil || parsedOrigin.Scheme == "" || parsedOrigin.Host == "" {
				return fmt.Errorf("RHYTHMIQ_ALLOWED_ORIGINS contains invalid URL %q", origin)
			}
			if !strings.EqualFold(parsedOrigin.Scheme, "https") {
				return fmt.Errorf("RHYTHMIQ_ALLOWED_ORIGINS must use https origins for public deployments")
			}
			if parsedOrigin.Path != "" && parsedOrigin.Path != "/" {
				return fmt.Errorf("RHYTHMIQ_ALLOWED_ORIGINS must not include path components (%q)", origin)
			}
			if parsedOrigin.RawQuery != "" || parsedOrigin.Fragment != "" || parsedOrigin.User != nil {
				return fmt.Errorf("RHYTHMIQ_ALLOWED_ORIGINS must only include scheme+host (%q)", origin)
			}

			originHost := strings.ToLower(parsedOrigin.Hostname())
			if originHost == "localhost" || originHost == "127.0.0.1" || originHost == "::1" {
				return fmt.Errorf("RHYTHMIQ_ALLOWED_ORIGINS must not include loopback origins for public deployments")
			}
		}
	}

	return nil
}

func startAutoSnapshotCollector(repo db.Repository, metrics *service.MetricsService) {
	const syncInterval = 6 * time.Hour
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		userIDs, err := repo.ListUserIDs(ctx)
		if err != nil {
			log.Printf("auto snapshot user list failed: %v", err)
			return
		}

		for _, userID := range userIDs {
			if _, err := metrics.RefreshSnapshot(ctx, userID); err != nil {
				log.Printf("auto snapshot refresh failed for user %s: %v", userID, err)
			}
		}
	}

	time.Sleep(10 * time.Second)
	run()
	for range ticker.C {
		run()
	}
}
