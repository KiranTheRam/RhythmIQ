package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"rhythmiq/internal/config"
	"rhythmiq/internal/db"
	"rhythmiq/internal/service"
	"rhythmiq/internal/spotify"
	"rhythmiq/internal/util"
)

const oauthStateCookieName = "rhythmiq_spotify_oauth_state"

// Server hosts all HTTP handlers.
type Server struct {
	cfg          config.Config
	repo         db.Repository
	spotify      *spotify.Client
	metrics      *service.MetricsService
	recommenders *service.RecommendationService
	sessions     *sessionManager
	authLimiter  *rateLimiter
	dataLimiter  *rateLimiter
	aiLimiter    *rateLimiter
}

// NewServer builds server dependencies.
func NewServer(cfg config.Config, repo db.Repository, spotifyClient *spotify.Client, metrics *service.MetricsService, recommenders *service.RecommendationService) *Server {
	return &Server{
		cfg:          cfg,
		repo:         repo,
		spotify:      spotifyClient,
		metrics:      metrics,
		recommenders: recommenders,
		sessions:     newSessionManager(cfg.SessionSecret, cfg.ForceSecureCookies),
		authLimiter:  newRateLimiter(30, time.Minute),
		dataLimiter:  newRateLimiter(20, time.Minute),
		aiLimiter:    newRateLimiter(12, time.Minute),
	}
}

// Routes returns an http handler with all API endpoints.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeadersMiddleware(s.cfg.ForceSecureCookies))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", s.handleHealth)
	r.Get("/auth/status", s.handleAuthStatus)
	r.With(rateLimitMiddleware(s.authLimiter)).Get("/auth/login", s.handleAuthLogin)
	r.With(rateLimitMiddleware(s.authLimiter)).Get("/auth/callback", s.handleAuthCallback)
	r.Post("/auth/logout", s.handleLogout)

	r.With(rateLimitMiddleware(s.dataLimiter)).Post("/metrics/refresh", s.handleRefreshMetrics)
	r.Get("/metrics/latest", s.handleLatestMetrics)
	r.Get("/metrics/history", s.handleHistory)

	r.With(rateLimitMiddleware(s.aiLimiter)).Get("/recommendations/insights", s.handleRecommendations)

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"service":           "rhythmiq",
		"spotifyConfigured": s.spotify.IsConfigured(),
	})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if !s.spotify.IsConfigured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated":     false,
			"spotifyConfigured": false,
		})
		return
	}

	userID, err := s.sessions.userID(r)
	if err != nil {
		if errors.Is(err, errNoSession) || errors.Is(err, errInvalidSession) {
			if errors.Is(err, errInvalidSession) {
				s.sessions.clear(w, r)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"authenticated":     false,
				"spotifyConfigured": true,
			})
			return
		}
		writeInternalError(w, r, "failed to load active session", err)
		return
	}

	profile, err := s.repo.GetUserProfile(r.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			s.sessions.clear(w, r)
			writeJSON(w, http.StatusOK, map[string]any{
				"authenticated":     false,
				"spotifyConfigured": true,
			})
			return
		}
		writeInternalError(w, r, "failed to load profile", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":     true,
		"spotifyConfigured": true,
		"profile":           profile,
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if !s.spotify.IsConfigured() {
		writeError(w, http.StatusBadRequest, "Spotify is not configured. Set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET.")
		return
	}

	if canonical := s.canonicalAuthLoginURL(); canonical != "" && !sameHostPort(r.Host, canonical) {
		http.Redirect(w, r, canonical, http.StatusFound)
		return
	}

	state, err := util.RandomString(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate oauth state")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	http.Redirect(w, r, s.spotify.AuthURL(state), http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if !s.spotify.IsConfigured() {
		writeError(w, http.StatusBadRequest, "Spotify is not configured")
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "missing spotify callback parameters")
		return
	}

	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || cookie.Value == "" || cookie.Value != state {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}

	token, err := s.spotify.ExchangeCode(r.Context(), code)
	if err != nil {
		writeUpstreamError(w, r, "failed to exchange Spotify OAuth code", err)
		return
	}

	profile, err := s.spotify.GetCurrentUser(r.Context(), token.AccessToken)
	if err != nil {
		writeUpstreamError(w, r, "failed to load Spotify profile", err)
		return
	}

	if err := s.repo.UpsertUserProfile(r.Context(), profile); err != nil {
		writeInternalError(w, r, "failed to persist user profile", err)
		return
	}
	if err := s.repo.SaveToken(r.Context(), profile.ID, token); err != nil {
		writeInternalError(w, r, "failed to persist Spotify token", err)
		return
	}
	if err := s.sessions.setUserID(w, r, profile.ID); err != nil {
		writeInternalError(w, r, "failed to set session user", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	_, _ = s.metrics.RefreshSnapshot(ctx, profile.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	http.Redirect(w, r, "/?auth=success", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.clear(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRefreshMetrics(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireActiveUser(w, r)
	if !ok {
		return
	}

	snapshot, err := s.metrics.RefreshSnapshot(r.Context(), userID)
	if err != nil {
		writeUpstreamError(w, r, "failed to refresh metrics", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleLatestMetrics(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireActiveUser(w, r)
	if !ok {
		return
	}

	snapshot, err := s.metrics.GetLatestSnapshot(r.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no snapshots found, run a refresh first")
			return
		}
		writeInternalError(w, r, "failed to load latest snapshot", err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireActiveUser(w, r)
	if !ok {
		return
	}

	days := 90
	if v := r.URL.Query().Get("days"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 366 {
			days = parsed
		}
	}

	points, err := s.metrics.GetHistory(r.Context(), userID, days)
	if err != nil {
		writeInternalError(w, r, "failed to load history", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":   days,
		"points": points,
	})
}

func (s *Server) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireActiveUser(w, r)
	if !ok {
		return
	}

	snapshot, err := s.metrics.GetLatestSnapshot(r.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no snapshots found, run a refresh first")
			return
		}
		writeInternalError(w, r, "failed to load latest snapshot", err)
		return
	}

	history, err := s.metrics.GetHistory(r.Context(), userID, 180)
	if err != nil {
		writeInternalError(w, r, "failed to load history for recommendations", err)
		return
	}

	insights, err := s.recommenders.GenerateInsights(r.Context(), snapshot, history)
	if err != nil {
		writeInternalError(w, r, "failed to generate recommendations", err)
		return
	}
	writeJSON(w, http.StatusOK, insights)
}

func (s *Server) requireActiveUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, err := s.sessions.userID(r)
	if err != nil || userID == "" {
		if errors.Is(err, errInvalidSession) {
			s.sessions.clear(w, r)
		}
		writeError(w, http.StatusUnauthorized, "spotify auth required")
		return "", false
	}
	return userID, true
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{"error": message})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeInternalError(w http.ResponseWriter, r *http.Request, action string, err error) {
	log.Printf("api internal error [%s %s] %s: %v", r.Method, r.URL.Path, action, err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeUpstreamError(w http.ResponseWriter, r *http.Request, action string, err error) {
	log.Printf("api upstream error [%s %s] %s: %v", r.Method, r.URL.Path, action, err)
	writeError(w, http.StatusBadGateway, "upstream service error")
}

func (s *Server) canonicalAuthLoginURL() string {
	parsed, err := url.Parse(s.cfg.SpotifyRedirectURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s/api/auth/login", parsed.Scheme, parsed.Host)
}

func sameHostPort(requestHost, targetURL string) bool {
	target, err := url.Parse(targetURL)
	if err != nil {
		return true
	}
	return strings.EqualFold(requestHost, target.Host)
}

func (s *Server) secureCookie(r *http.Request) bool {
	if s.cfg.ForceSecureCookies {
		return true
	}
	return isSecureRequest(r)
}
