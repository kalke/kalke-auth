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

// upstreamProxy copies an authenticated session request to a sibling API with M2M + user-forward.
type upstreamProxy struct {
	baseURL        string
	unavailableMsg string
	client         *http.Client
	token          func(context.Context) (string, error)
	forwardSecret  string
	allow          func([]string) bool
	logLabel       string
}

func (s *Server) proxyUpstream(w http.ResponseWriter, r *http.Request, path string, p upstreamProxy) {
	if p.baseURL == "" {
		writeErr(w, http.StatusServiceUnavailable, p.unavailableMsg)
		return
	}
	prin, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms := s.effectivePermissions(prin.UserEmail, prin.Permissions)
	if !p.allow(perms) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	bearer, err := p.token(r.Context())
	if err != nil {
		s.log.Error(p.logLabel+" m2m token", "err", err, "email", prin.UserEmail)
		writeErr(w, http.StatusBadGateway, "upstream auth failed")
		return
	}

	upstream := p.baseURL + path
	if q := r.URL.RawQuery; q != "" {
		upstream += "?" + q
	}
	var body io.Reader
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		body = r.Body
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstream, body) // #nosec G704
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "proxy error")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if idem := strings.TrimSpace(r.Header.Get("Idempotency-Key")); idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.ContentLength = r.ContentLength
	req.Header.Set("X-Kalke-User-Email", prin.UserEmail)
	req.Header.Set("X-Kalke-User-Sub", prin.UserSub)
	if ip := clientIP(r); ip != "" {
		req.Header.Set("X-Kalke-Client-IP", ip)
	}
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		req.Header.Set("X-Kalke-User-Agent", ua)
	}
	if p.forwardSecret != "" {
		req.Header.Set("X-Kalke-Forward-Secret", p.forwardSecret)
	}

	resp, err := p.client.Do(req) // #nosec G704
	if err != nil {
		s.log.Error(p.logLabel+" proxy", "err", err, "email", prin.UserEmail, "path", path)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for _, h := range []string{
		"Content-Type", "Cache-Control", "Retry-After",
		"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-Request-ID",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 8<<20))
}

func (s *Server) pdeUpstream() upstreamProxy {
	return upstreamProxy{
		baseURL:        s.cfg.PDEBaseURL,
		unavailableMsg: "extract proxy not configured",
		client:         s.pdeHTTP,
		token:          s.cachedM2MToken,
		forwardSecret:  s.cfg.PDEUserForwardSecret,
		allow:          func(perms []string) bool { return hasPermission(perms, "extract:write") },
		logLabel:       "pde",
	}
}

func (s *Server) ebankUpstream() upstreamProxy {
	return upstreamProxy{
		baseURL:        s.cfg.EbankBaseURL,
		unavailableMsg: "bank proxy not configured",
		client:         s.ebankHTTP,
		token:          s.cachedEbankM2MToken,
		forwardSecret:  s.cfg.EbankUserForwardSecret,
		allow:          hasBankPermission,
		logLabel:       "ebank",
	}
}

func (s *Server) proxyExtract(w http.ResponseWriter, r *http.Request) {
	s.proxyUpstream(w, r, "/v1/extract", s.pdeUpstream())
}

func (s *Server) proxyExtractions(w http.ResponseWriter, r *http.Request) {
	path := "/v1/extractions"
	if id := r.PathValue("id"); id != "" {
		path += "/" + id
	}
	s.proxyUpstream(w, r, path, s.pdeUpstream())
}

func (s *Server) bankProxy(upstreamPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.proxyUpstream(w, r, upstreamPath, s.ebankUpstream())
	}
}

// joinBankProxyPath appends display or cep path params onto an upstream prefix.
func joinBankProxyPath(upstreamPrefix, display, cep string) string {
	suffix := display
	if suffix == "" {
		suffix = cep
	}
	return strings.TrimRight(upstreamPrefix, "/") + "/" + suffix
}

// bankProxyPath appends a path parameter onto a fixed upstream prefix.
func (s *Server) bankProxyPath(upstreamPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := joinBankProxyPath(upstreamPrefix, r.PathValue("display"), r.PathValue("cep"))
		s.proxyUpstream(w, r, path, s.ebankUpstream())
	}
}

func (s *Server) cachedM2MToken(ctx context.Context) (string, error) {
	return s.cachedClientToken(ctx, &s.m2m, s.cfg.PDEM2MClientID, s.cfg.PDEM2MClientSecret)
}

func (s *Server) cachedEbankM2MToken(ctx context.Context) (string, error) {
	return s.cachedClientToken(ctx, &s.ebankM2M, s.cfg.EbankM2MClientID, s.cfg.EbankM2MClientSecret)
}

func (s *Server) cachedClientToken(ctx context.Context, cache *m2mCache, clientID, clientSecret string) (string, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.token != "" && time.Now().Before(cache.expiry) {
		return cache.token, nil
	}
	token, ttl, err := s.kc.ClientCredentialsToken(ctx, clientID, clientSecret)
	if err != nil {
		return "", err
	}
	skew := 30 * time.Second
	if ttl > 2*skew {
		ttl -= skew
	}
	cache.token = token
	cache.expiry = time.Now().Add(ttl)
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

func hasBankPermission(perms []string) bool {
	for _, p := range perms {
		if p == "bank:write" || p == "bank:demo" || p == "admin" {
			return true
		}
	}
	return false
}
