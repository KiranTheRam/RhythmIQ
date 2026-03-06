package api

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	defaultRateLimiterMaxEntries    = 20000
	defaultRateLimiterCleanupPeriod = 5 * time.Minute
)

type rateLimiter struct {
	mu             sync.Mutex
	limit          int
	window         time.Duration
	entries        map[string]rateLimitEntry
	maxEntries     int
	cleanupPeriod  time.Duration
	nextCleanupRun time.Time
}

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	now := time.Now()
	return &rateLimiter{
		limit:          limit,
		window:         window,
		entries:        make(map[string]rateLimitEntry),
		maxEntries:     defaultRateLimiterMaxEntries,
		cleanupPeriod:  defaultRateLimiterCleanupPeriod,
		nextCleanupRun: now.Add(defaultRateLimiterCleanupPeriod),
	}
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maybeCleanup(now)

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= l.window {
		l.entries[key] = rateLimitEntry{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if entry.count >= l.limit {
		return false
	}

	entry.count++
	l.entries[key] = entry
	return true
}

func (l *rateLimiter) maybeCleanup(now time.Time) {
	if len(l.entries) < l.maxEntries && now.Before(l.nextCleanupRun) {
		return
	}

	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= l.window {
			delete(l.entries, key)
		}
	}

	// Keep memory bounded even under high-cardinality attack traffic.
	if len(l.entries) > l.maxEntries {
		overflow := len(l.entries) - l.maxEntries
		for key := range l.entries {
			delete(l.entries, key)
			overflow--
			if overflow <= 0 {
				break
			}
		}
	}

	l.nextCleanupRun = now.Add(l.cleanupPeriod)
}

func rateLimitMiddleware(l *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil {
				next.ServeHTTP(w, r)
				return
			}

			now := time.Now()
			if !l.allow(clientIP(r), now) {
				w.Header().Set("Retry-After", strconv.Itoa(int(l.window.Seconds())))
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
