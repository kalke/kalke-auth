package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type m2mCache struct {
	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (s *Server) proxyExtract(w http.ResponseWriter, r *http.Request) {
	s.proxyPDE(w, r, "/v1/extract")
}

func (s *Server) proxyExtractions(w http.ResponseWriter, r *http.Request) {
	path := "/v1/extractions"
	if id := r.PathValue("id"); id != "" {
		path += "/" + id
	}
	s.proxyPDE(w, r, path)
}

func (s *Server) proxyPDE(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	if s.cfg.PDEBaseURL == "" {
		writeErr(w, http.StatusServiceUnavailable, "extract proxy not configured")
		return
	}
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms := s.effectivePermissions(p.UserEmail, p.Permissions)
	if !hasPermission(perms, "extract:write") {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	bearer, err := s.cachedM2MToken(r.Context())
	if err != nil {
		s.log.Error("pde m2m token", "err", err, "email", p.UserEmail)
		writeErr(w, http.StatusBadGateway, "upstream auth failed")
		return
	}

	upstream := s.cfg.PDEBaseURL + upstreamPath
	if q := r.URL.RawQuery; q != "" {
		upstream += "?" + q
	}
	var body io.Reader
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		body = r.Body
	}
	// PDEBaseURL is server config; path is a fixed allowlist from route handlers.
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, body) // #nosec G704
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "proxy error")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.ContentLength = r.ContentLength
	req.Header.Set("X-Kalke-User-Email", p.UserEmail)
	req.Header.Set("X-Kalke-User-Sub", p.UserSub)
	// Browser origin — PDE would otherwise see the BFF's IP and Go-http-client UA.
	if ip := clientIP(r); ip != "" {
		req.Header.Set("X-Kalke-Client-IP", ip)
	}
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		req.Header.Set("X-Kalke-User-Agent", ua)
	}
	if s.cfg.PDEUserForwardSecret != "" {
		req.Header.Set("X-Kalke-Forward-Secret", s.cfg.PDEUserForwardSecret)
	}

	resp, err := s.pdeHTTP.Do(req) // #nosec G704
	if err != nil {
		s.log.Error("pde proxy", "err", err, "email", p.UserEmail, "path", upstreamPath)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for _, h := range []string{"Content-Type", "Cache-Control", "Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 8<<20))
}

func (s *Server) cachedM2MToken(ctx context.Context) (string, error) {
	s.m2m.mu.Lock()
	defer s.m2m.mu.Unlock()
	if s.m2m.token != "" && time.Now().Before(s.m2m.expiry) {
		return s.m2m.token, nil
	}
	token, ttl, err := s.kc.ClientCredentialsToken(ctx, s.cfg.PDEM2MClientID, s.cfg.PDEM2MClientSecret)
	if err != nil {
		return "", err
	}
	// Refresh a bit before expiry.
	skew := 30 * time.Second
	if ttl > 2*skew {
		ttl -= skew
	}
	s.m2m.token = token
	s.m2m.expiry = time.Now().Add(ttl)
	return token, nil
}

func hasPermission(perms []string, want string) bool {
	for _, p := range perms {
		if p == want || p == "admin" {
			return true
		}
	}
	return false
}
