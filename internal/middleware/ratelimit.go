package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// visitor tracks request counts for a single IP address.
type visitor struct {
	count    int
	windowStart time.Time
}

// RateLimiter provides IP-based rate limiting using a sliding window counter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int           // max requests per window
	window   time.Duration // window duration
	done     chan struct{}
}

// NewRateLimiter creates a new rate limiter with the given requests-per-minute limit.
// It starts a background goroutine that cleans up stale entries every 5 minutes.
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    requestsPerMinute,
		window:   time.Minute,
		done:     make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl
}

// Middleware returns an HTTP middleware that enforces rate limiting.
// If the limit is exceeded, it returns 429 Too Many Requests with a Retry-After header.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		if !rl.allow(ip) {
			retryAfter := int(rl.window.Seconds())
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			slog.Warn("rate limit exceeded", "ip", ip)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allow checks whether the given IP is within the rate limit.
// Returns true if the request is allowed, false if rate limited.
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]

	if !exists || now.Sub(v.windowStart) >= rl.window {
		// New visitor or window has expired — reset
		rl.visitors[ip] = &visitor{
			count:    1,
			windowStart: now,
		}
		return true
	}

	// Within the current window
	v.count++
	return v.count <= rl.limit
}

// cleanupLoop periodically removes stale visitor entries.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.done:
			return
		}
	}
}

// cleanup removes visitor entries whose window has expired.
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, v := range rl.visitors {
		if now.Sub(v.windowStart) >= rl.window {
			delete(rl.visitors, ip)
		}
	}
}

// Stop shuts down the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.done)
}

// extractIP attempts to get the real client IP from common proxy headers,
// falling back to RemoteAddr.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For first (set by reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr (includes port, but that's fine for rate limiting)
	return r.RemoteAddr
}
