package httpserver

import (
	"net/http"
	"sync"
	"time"
)

type rateEntry struct {
	window time.Time
	count  int
}

type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
	limit   int
	window  time.Duration
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		entries: map[string]rateEntry{}, limit: limit, window: window,
	}
}

func (limiter *ipRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		limiter.mu.Lock()
		entry := limiter.entries[r.RemoteAddr]
		if entry.window.IsZero() || now.Sub(entry.window) >= limiter.window {
			entry = rateEntry{window: now}
		}
		entry.count++
		limiter.entries[r.RemoteAddr] = entry
		allowed := entry.count <= limiter.limit
		if len(limiter.entries) > 10_000 {
			for key, current := range limiter.entries {
				if now.Sub(current.window) >= limiter.window {
					delete(limiter.entries, key)
				}
			}
		}
		limiter.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
