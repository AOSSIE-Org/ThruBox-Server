package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		proxies    int
		remoteAddr string
		xff        []string // one entry per X-Forwarded-For header line
		realIP     string
		want       string
	}{
		{
			name:       "no proxies uses the connecting address",
			remoteAddr: "203.0.113.7:51234",
			want:       "203.0.113.7",
		},
		{
			name:       "no proxies ignores a spoofed X-Forwarded-For",
			remoteAddr: "203.0.113.7:51234",
			xff:        []string{"9.9.9.9"},
			want:       "203.0.113.7",
		},
		{
			name:       "no proxies ignores a spoofed X-Real-IP",
			remoteAddr: "203.0.113.7:51234",
			realIP:     "9.9.9.9",
			want:       "203.0.113.7",
		},
		{
			name:       "one proxy reads the rightmost entry",
			proxies:    1,
			remoteAddr: "10.0.0.1:443",
			xff:        []string{"198.51.100.4"},
			want:       "198.51.100.4",
		},
		{
			name:       "one proxy skips entries the client prepended",
			proxies:    1,
			remoteAddr: "10.0.0.1:443",
			xff:        []string{"9.9.9.9, 1.2.3.4, 198.51.100.4"},
			want:       "198.51.100.4",
		},
		{
			name:       "two proxies read the second entry from the right",
			proxies:    2,
			remoteAddr: "10.0.0.2:443",
			xff:        []string{"9.9.9.9, 198.51.100.4, 10.0.0.1"},
			want:       "198.51.100.4",
		},
		{
			name:       "multiple header lines form one list",
			proxies:    2,
			remoteAddr: "10.0.0.2:443",
			xff:        []string{"9.9.9.9, 198.51.100.4", "10.0.0.1"},
			want:       "198.51.100.4",
		},
		{
			name:       "too few entries falls back to the connecting address",
			proxies:    2,
			remoteAddr: "10.0.0.2:443",
			xff:        []string{"198.51.100.4"},
			want:       "10.0.0.2",
		},
		{
			name:       "missing header falls back to the connecting address",
			proxies:    1,
			remoteAddr: "10.0.0.1:443",
			want:       "10.0.0.1",
		},
		{
			name:       "garbage entry falls back to the connecting address",
			proxies:    1,
			remoteAddr: "10.0.0.1:443",
			xff:        []string{"not-an-ip"},
			want:       "10.0.0.1",
		},
		{
			name:       "IPv6 entry is accepted",
			proxies:    1,
			remoteAddr: "[::1]:443",
			xff:        []string{"2001:db8::1"},
			want:       "2001:db8::1",
		},
		{
			name:       "connecting address without a port is used as is",
			remoteAddr: "203.0.113.7",
			want:       "203.0.113.7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/messages/alice", nil)
			r.RemoteAddr = tc.remoteAddr
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}

			if got := clientIP(r, tc.proxies); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRateLimiter_RotatingXFFIsStillLimited reproduces the bypass: a client
// sending a fresh X-Forwarded-For on every request used to get a fresh
// bucket each time and was never limited.
func TestRateLimiter_RotatingXFFIsStillLimited(t *testing.T) {
	for _, proxies := range []int{0, 1} {
		t.Run(fmt.Sprintf("trusted_proxies=%d", proxies), func(t *testing.T) {
			const limit = 5
			rl := NewRateLimiter(limit, proxies)
			defer rl.Stop()

			reached := false
			h := rl.Middleware(okHandler(&reached))

			limited := 0
			for i := 0; i < limit+3; i++ {
				r := httptest.NewRequest(http.MethodGet, "/api/messages/alice", nil)
				r.RemoteAddr = "10.0.0.1:443"
				// The client's spoofed value, then the real client IP that a
				// trusted proxy appended.
				r.Header.Set("X-Forwarded-For", fmt.Sprintf("10.9.9.%d, 198.51.100.4", i))

				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, r)
				if rec.Code == http.StatusTooManyRequests {
					limited++
				}
			}

			if limited != 3 {
				t.Errorf("%d of %d requests limited, want 3", limited, limit+3)
			}
		})
	}
}

// TestRateLimiter_SeparateClientsBehindProxy checks that, with the proxy
// configured, two clients behind the same proxy get separate buckets.
func TestRateLimiter_SeparateClientsBehindProxy(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Stop()

	reached := false
	h := rl.Middleware(okHandler(&reached))

	for _, client := range []string{"198.51.100.4", "198.51.100.5"} {
		r := httptest.NewRequest(http.MethodGet, "/api/messages/alice", nil)
		r.RemoteAddr = "10.0.0.1:443"
		r.Header.Set("X-Forwarded-For", client)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Errorf("client %s: status %d, want 200", client, rec.Code)
		}
	}
}
