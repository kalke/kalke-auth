package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kalke/kalke-auth/internal/config"
)

func TestClientIPTrustsXRealIPOnlyFromPrivateHop(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.2:443"
	req.Header.Set("X-Real-IP", "203.0.113.9")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "9.9.9.9")
	if got := clientIP(req); got != "203.0.113.9" {
		t.Fatalf("got %q want 203.0.113.9", got)
	}
}

func TestClientIPIgnoresSpoofedHeadersFromPublicHop(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.50:443"
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("CF-Connecting-IP", "9.9.9.9")
	if got := clientIP(req); got != "203.0.113.50" {
		t.Fatalf("got %q want 203.0.113.50", got)
	}
}

func TestRequireAllowedOrigin(t *testing.T) {
	s := &Server{cfg: config.Config{CORSOrigins: []string{"https://kalke.dev"}}}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := s.requireAllowedOrigin(inner)

	post := func(origin, referer string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := post("", ""); code != http.StatusNoContent {
		t.Fatalf("missing origin: %d", code)
	}
	if code := post("https://kalke.dev", ""); code != http.StatusNoContent {
		t.Fatalf("allowed origin: %d", code)
	}
	if code := post("https://evil.example", ""); code != http.StatusForbidden {
		t.Fatalf("evil origin: %d", code)
	}
	if code := post("", "https://evil.example/phish"); code != http.StatusForbidden {
		t.Fatalf("evil referer: %d", code)
	}
	if code := post("", "https://kalke.dev/playground"); code != http.StatusNoContent {
		t.Fatalf("allowed referer: %d", code)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/me", nil)
	req.Header.Set("Origin", "https://evil.example")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("GET should skip origin check, got %d", rec.Code)
	}
}
