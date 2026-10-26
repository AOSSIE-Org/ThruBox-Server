package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// visitor tracks request counts for a single IP address.
type visitor struct {
	count       int
	windowStart time.Time
}

// RateLimiter provides IP-based rate limiting using a sliding window counter.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    int           // max requests per window
	window   time.Duration // window duration
	proxies  int           // trusted reverse proxies in front of the relay
	done     chan struct{}
}

// NewRateLimiter creates a new rate limiter with the given requests-per-minute limit.
// trustedProxies is the number of reverse proxies in front of the relay; it
// decides how the client IP is read (see clientIP).
// It starts a background goroutine that cleans up stale entries every 1 minute.
func NewRateLimiter(requestsPerMinute, trustedProxies int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		limit:    requestsPerMinute,
		window:   time.Minute,
		proxies:  trustedProxies,
		done:     make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl
}

// Middleware returns an HTTP middleware that enforces rate limiting.
// If the limit is exceeded, it returns 429 Too Many Requests with a Retry-After header.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, rl.proxies)

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
			count:       1,
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
	ticker := time.NewTicker(1 * time.Minute)
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

// clientIP returns the address to rate-limit r by.
//
// With no trusted proxies the connecting address is the only thing a client
// cannot forge, so forwarding headers are ignored entirely.
//
// Behind trustedProxies proxies, each proxy appends the address it received
// the request from to X-Forwarded-For. The last trustedProxies entries were
// therefore written by proxies we trust, and the entry at that position from
// the right is the client as seen by the outermost proxy. Anything to its left
// came from the client and may be spoofed, so it is never read. X-Real-IP is
// not consulted: it is a single value with no way to tell who set it.
//
// If the header has fewer entries than expected, or the entry is not an IP,
// the request did not come through the configured proxy chain and we fall back
// to the connecting address.
func clientIP(r *http.Request, trustedProxies int) string {
	if trustedProxies > 0 {
		// A request may carry several X-Forwarded-For headers; together they
		// form one list in order.
		var hops []string
		for _, h := range r.Header.Values("X-Forwarded-For") {
			for _, part := range strings.Split(h, ",") {
				hops = append(hops, strings.TrimSpace(part))
			}
		}
		if len(hops) >= trustedProxies {
			if ip := net.ParseIP(hops[len(hops)-trustedProxies]); ip != nil {
				return ip.String()
			}
		}
	}

	// Fall back to RemoteAddr — strip the port
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
