package middleware

import (
	"net/http"
	"strconv"
	"strings"
)

// preflightMaxAge is how long a browser may cache a preflight result, in
// seconds. Ten minutes keeps preflight chatter down without making an origin
// allowlist change take long to propagate.
const preflightMaxAge = 600

// corsWildcard allows any origin. Configuring it is opt-in.
const corsWildcard = "*"

// allowedMethods mirrors the routes registered in cmd/relay: GET /health,
// POST /api/messages, GET /api/messages/{address}, DELETE /api/messages/{id}.
var allowedMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodDelete,
	http.MethodOptions,
}

// CORS returns an HTTP middleware that serves cross-origin headers for the
// origins in allowOrigins.
//
// If allowOrigins is empty the middleware is a no-op passthrough, which is the
// default: the relay serves no CORS headers and OPTIONS keeps 404ing from the
// router, exactly as before. Operators opt in by listing origins, or by
// listing "*" to allow any.
//
// requireAPIKey adds X-API-Key to Access-Control-Allow-Headers, so browsers
// are permitted to send it when the relay has an API key configured.
//
// This middleware must sit OUTERMOST in the chain, ahead of APIKeyAuth.
// Browsers never attach custom headers to a preflight, so an OPTIONS request
// carries no X-API-Key; if authentication ran first every preflight would 401
// and the real request would never be sent.
func CORS(allowOrigins []string, requireAPIKey bool) func(http.Handler) http.Handler {
	allowed, wildcard := normalizeOrigins(allowOrigins)

	allowHeaders := "Content-Type"
	if requireAPIKey {
		allowHeaders = "Content-Type, X-API-Key"
	}
	allowMethods := strings.Join(allowedMethods, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Not configured — behave exactly as the relay did before.
			if !wildcard && len(allowed) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")

			// Same-origin and non-browser callers (curl, the Go SDK,
			// server-to-server) send no Origin. Leave them untouched.
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			// The response now varies by request origin. Set this even when
			// the origin is rejected, so a shared cache cannot serve one
			// origin's response to another.
			w.Header().Add("Vary", "Origin")

			if !originAllowed(allowed, wildcard, origin) {
				// A preflight we will not honour is answered directly, so the
				// caller gets a clear status instead of a confusing 404 from
				// the router. No CORS headers are emitted, so the browser
				// blocks the real request either way.
				if isPreflight(r) {
					http.Error(w, "origin not allowed by CORS policy", http.StatusForbidden)
					return
				}
				// A non-preflight request from an unlisted origin is still
				// served; without the header the browser hides the response.
				next.ServeHTTP(w, r)
				return
			}

			// Echo the concrete origin rather than "*". It is required if a
			// caller ever uses credentials, and it keeps the response honest
			// about which origin it was built for.
			w.Header().Set("Access-Control-Allow-Origin", origin)

			if isPreflight(r) {
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(preflightMaxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isPreflight reports whether r is a CORS preflight: an OPTIONS request
// carrying Access-Control-Request-Method. A bare OPTIONS is not a preflight
// and is left to the router.
func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions &&
		r.Header.Get("Access-Control-Request-Method") != ""
}

// normalizeOrigins trims, lowercases and de-duplicates the configured origins,
// dropping empties and any trailing slash (a common config slip — an Origin
// header never has one). It reports whether the wildcard was present.
func normalizeOrigins(origins []string) (map[string]struct{}, bool) {
	allowed := make(map[string]struct{}, len(origins))
	wildcard := false

	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == corsWildcard {
			wildcard = true
			continue
		}
		allowed[strings.ToLower(strings.TrimSuffix(o, "/"))] = struct{}{}
	}

	return allowed, wildcard
}

// originAllowed reports whether origin passes the allowlist.
func originAllowed(allowed map[string]struct{}, wildcard bool, origin string) bool {
	if wildcard {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimSuffix(origin, "/"))]
	return ok
}
