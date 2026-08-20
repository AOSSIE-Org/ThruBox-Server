package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// okHandler records whether the request reached the end of the chain.
func okHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// preflightReq builds a browser-style CORS preflight for the given origin.
func preflightReq(origin, method string) *http.Request {
	r := httptest.NewRequest(http.MethodOptions, "/api/messages", nil)
	r.Header.Set("Origin", origin)
	r.Header.Set("Access-Control-Request-Method", method)
	return r
}

// simpleReq builds an ordinary cross-origin request.
func simpleReq(origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/messages/alice", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestCORS_DisabledByDefault(t *testing.T) {
	reached := false
	h := CORS(nil, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq("https://app.example.com"))

	if !reached {
		t.Error("request did not reach the next handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty when CORS is off", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q, want empty when CORS is off", got)
	}
}

func TestCORS_DisabledLeavesPreflightToTheRouter(t *testing.T) {
	reached := false
	h := CORS(nil, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflightReq("https://app.example.com", http.MethodPost))

	if !reached {
		t.Error("preflight was intercepted even though CORS is disabled")
	}
}

func TestCORS_AllowedOriginOnSimpleRequest(t *testing.T) {
	reached := false
	h := CORS([]string{"https://app.example.com"}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq("https://app.example.com"))

	if !reached {
		t.Error("request did not reach the next handler")
	}
	if got, want := rec.Header().Get("Access-Control-Allow-Origin"), "https://app.example.com"; got != want {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, want)
	}
	if got, want := rec.Header().Get("Vary"), "Origin"; got != want {
		t.Errorf("Vary = %q, want %q", got, want)
	}
}

func TestCORS_PreflightIsAnsweredDirectly(t *testing.T) {
	reached := false
	h := CORS([]string{"https://app.example.com"}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflightReq("https://app.example.com", http.MethodPost))

	if reached {
		t.Error("preflight reached the next handler; it should be answered by the middleware")
	}
	if got, want := rec.Code, http.StatusNoContent; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if got, want := rec.Header().Get("Access-Control-Allow-Origin"), "https://app.example.com"; got != want {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, want)
	}

	methods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"GET", "POST", "DELETE"} {
		if !strings.Contains(methods, m) {
			t.Errorf("Access-Control-Allow-Methods = %q, want it to include %s", methods, m)
		}
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Error("Access-Control-Max-Age is not set")
	}
}

func TestCORS_AllowHeadersTracksAPIKey(t *testing.T) {
	tests := []struct {
		name          string
		requireAPIKey bool
		wantAPIKey    bool
	}{
		{name: "no api key configured", requireAPIKey: false, wantAPIKey: false},
		{name: "api key configured", requireAPIKey: true, wantAPIKey: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			h := CORS([]string{"https://app.example.com"}, tt.requireAPIKey)(okHandler(&reached))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, preflightReq("https://app.example.com", http.MethodPost))

			got := rec.Header().Get("Access-Control-Allow-Headers")
			if !strings.Contains(got, "Content-Type") {
				t.Errorf("Access-Control-Allow-Headers = %q, want it to include Content-Type", got)
			}
			if hasKey := strings.Contains(got, "X-API-Key"); hasKey != tt.wantAPIKey {
				t.Errorf("Access-Control-Allow-Headers = %q, X-API-Key present = %v, want %v",
					got, hasKey, tt.wantAPIKey)
			}
		})
	}
}

func TestCORS_DisallowedOriginPreflightIsRejected(t *testing.T) {
	reached := false
	h := CORS([]string{"https://app.example.com"}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflightReq("https://evil.example.com", http.MethodPost))

	if reached {
		t.Error("rejected preflight reached the next handler")
	}
	if got, want := rec.Code, http.StatusForbidden; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for a disallowed origin", got)
	}
	if got, want := rec.Header().Get("Vary"), "Origin"; got != want {
		t.Errorf("Vary = %q, want %q even on rejection, to keep shared caches honest", got, want)
	}
}

func TestCORS_DisallowedOriginSimpleRequestGetsNoHeader(t *testing.T) {
	reached := false
	h := CORS([]string{"https://app.example.com"}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq("https://evil.example.com"))

	if !reached {
		t.Error("non-preflight request should still be served")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty; the browser must block the read", got)
	}
}

func TestCORS_NoOriginHeaderIsUntouched(t *testing.T) {
	reached := false
	h := CORS([]string{"https://app.example.com"}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq(""))

	if !reached {
		t.Error("request without Origin did not reach the next handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for a non-browser caller", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q, want empty when no Origin was sent", got)
	}
}

func TestCORS_Wildcard(t *testing.T) {
	reached := false
	h := CORS([]string{"*"}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq("https://anything.example.com"))

	if got, want := rec.Header().Get("Access-Control-Allow-Origin"), "https://anything.example.com"; got != want {
		t.Errorf("Access-Control-Allow-Origin = %q, want the echoed origin %q", got, want)
	}
}

func TestCORS_NeverAllowsCredentials(t *testing.T) {
	// Credentialed CORS is not supported: the relay authenticates with a
	// header, not cookies, and advertising credentials alongside "*" would be
	// a footgun.
	reached := false
	h := CORS([]string{"*"}, true)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflightReq("https://anything.example.com", http.MethodPost))

	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want it never to be set", got)
	}
}

func TestCORS_OriginNormalization(t *testing.T) {
	tests := []struct {
		name      string
		configure string
		request   string
		wantAllow bool
	}{
		{
			name: "exact match", configure: "https://app.example.com",
			request: "https://app.example.com", wantAllow: true,
		},
		{
			name: "configured with a trailing slash", configure: "https://app.example.com/",
			request: "https://app.example.com", wantAllow: true,
		},
		{
			name: "configured with surrounding spaces", configure: "  https://app.example.com  ",
			request: "https://app.example.com", wantAllow: true,
		},
		{
			name: "mixed case in config", configure: "https://APP.example.com",
			request: "https://app.example.com", wantAllow: true,
		},
		{
			name: "a different scheme is a different origin", configure: "https://app.example.com",
			request: "http://app.example.com", wantAllow: false,
		},
		{
			name: "a different port is a different origin", configure: "https://app.example.com",
			request: "https://app.example.com:8443", wantAllow: false,
		},
		{
			name: "suffix attack is not a match", configure: "https://app.example.com",
			request: "https://app.example.com.evil.tld", wantAllow: false,
		},
		{
			name: "subdomains are not implicitly allowed", configure: "https://example.com",
			request: "https://sub.example.com", wantAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			h := CORS([]string{tt.configure}, false)(okHandler(&reached))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, simpleReq(tt.request))

			allowed := rec.Header().Get("Access-Control-Allow-Origin") != ""
			if allowed != tt.wantAllow {
				t.Errorf("origin %q against config %q: allowed = %v, want %v",
					tt.request, tt.configure, allowed, tt.wantAllow)
			}
		})
	}
}

func TestCORS_BareOptionsIsNotAPreflight(t *testing.T) {
	// An OPTIONS request with no Access-Control-Request-Method is not a
	// preflight; it belongs to the router, not to this middleware.
	reached := false
	h := CORS([]string{"https://app.example.com"}, false)(okHandler(&reached))

	r := httptest.NewRequest(http.MethodOptions, "/api/messages", nil)
	r.Header.Set("Origin", "https://app.example.com")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if !reached {
		t.Error("bare OPTIONS was swallowed by the CORS middleware")
	}
}

func TestCORS_EmptyStringOriginsAreIgnored(t *testing.T) {
	// A config like RELAY_SECURITY_ALLOWED_ORIGINS="," must not become an
	// allowlist containing the empty origin.
	reached := false
	h := CORS([]string{"", "   "}, false)(okHandler(&reached))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq("https://evil.example.com"))

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
	if !reached {
		t.Error("request should pass through when the allowlist is effectively empty")
	}
}

// TestCORS_PreflightSurvivesAPIKeyAuth is the middleware-ordering regression
// test. Browsers never attach X-API-Key to a preflight, so CORS must answer it
// before APIKeyAuth has a chance to reject it.
func TestCORS_PreflightSurvivesAPIKeyAuth(t *testing.T) {
	reached := false

	var h http.Handler = okHandler(&reached)
	h = APIKeyAuth("secret-key")(h)
	h = CORS([]string{"https://app.example.com"}, true)(h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, preflightReq("https://app.example.com", http.MethodPost))

	if got, want := rec.Code, http.StatusNoContent; got != want {
		t.Errorf("preflight status = %d, want %d (401 means CORS is nested inside APIKeyAuth)", got, want)
	}
	if got, want := rec.Header().Get("Access-Control-Allow-Origin"), "https://app.example.com"; got != want {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, want)
	}
}

// TestCORS_ActualRequestStillNeedsTheAPIKey confirms CORS has not become an
// authentication bypass for real (non-preflight) requests.
func TestCORS_ActualRequestStillNeedsTheAPIKey(t *testing.T) {
	reached := false

	var h http.Handler = okHandler(&reached)
	h = APIKeyAuth("secret-key")(h)
	h = CORS([]string{"https://app.example.com"}, true)(h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, simpleReq("https://app.example.com"))

	if reached {
		t.Error("request without an API key reached the handler")
	}
	if got, want := rec.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}
