package httpapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) proxyBankBootstrap(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/demo/bootstrap")
}

func (s *Server) proxyBankMeta(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/demo/meta")
}

func (s *Server) proxyBankAccount(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/me/account")
}

func (s *Server) proxyBankTransactions(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/me/transactions")
}

func (s *Server) proxyBankTransfer(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/me/transfer")
}

func (s *Server) proxyBankWithdraw(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/me/withdraw")
}

func (s *Server) proxyBankOnboarding(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/onboarding")
}

func (s *Server) proxyBankOnboardingStart(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/onboarding/start")
}

func (s *Server) proxyBankOnboardingConsent(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/onboarding/consent")
}

func (s *Server) proxyBankOnboardingSkip(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/onboarding/skip")
}

func (s *Server) proxyBankOnboardingDocuments(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/onboarding/documents")
}

func (s *Server) proxyBankOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	s.proxyEbank(w, r, "/v1/onboarding/complete")
}

func (s *Server) proxyEbank(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	if s.cfg.EbankBaseURL == "" {
		writeErr(w, http.StatusServiceUnavailable, "bank proxy not configured")
		return
	}
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms := s.effectivePermissions(p.UserEmail, p.Permissions)
	if !hasBankPermission(perms) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	bearer, err := s.cachedEbankM2MToken(r.Context())
	if err != nil {
		s.log.Error("ebank m2m token", "err", err, "email", p.UserEmail)
		writeErr(w, http.StatusBadGateway, "upstream auth failed")
		return
	}

	upstream := s.cfg.EbankBaseURL + upstreamPath
	if q := r.URL.RawQuery; q != "" {
		upstream += "?" + q
	}
	var body io.Reader
	if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
		body = r.Body
	}
	// EbankBaseURL is server config; path is a fixed allowlist from route handlers.
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
	if ip := clientIP(r); ip != "" {
		req.Header.Set("X-Kalke-Client-IP", ip)
	}
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		req.Header.Set("X-Kalke-User-Agent", ua)
	}
	if s.cfg.EbankUserForwardSecret != "" {
		req.Header.Set("X-Kalke-Forward-Secret", s.cfg.EbankUserForwardSecret)
	}

	resp, err := s.ebankHTTP.Do(req) // #nosec G704
	if err != nil {
		s.log.Error("ebank proxy", "err", err, "email", p.UserEmail, "path", upstreamPath)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for _, h := range []string{
		"Content-Type",
		"Cache-Control",
		"Retry-After",
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-Request-ID",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 8<<20))
}

func (s *Server) cachedEbankM2MToken(ctx context.Context) (string, error) {
	s.ebankM2M.mu.Lock()
	defer s.ebankM2M.mu.Unlock()
	if s.ebankM2M.token != "" && time.Now().Before(s.ebankM2M.expiry) {
		return s.ebankM2M.token, nil
	}
	token, ttl, err := s.kc.ClientCredentialsToken(
		ctx,
		s.cfg.EbankM2MClientID,
		s.cfg.EbankM2MClientSecret,
	)
	if err != nil {
		return "", err
	}
	skew := 30 * time.Second
	if ttl > 2*skew {
		ttl -= skew
	}
	s.ebankM2M.token = token
	s.ebankM2M.expiry = time.Now().Add(ttl)
	return token, nil
}

func hasBankPermission(perms []string) bool {
	for _, p := range perms {
		if p == "bank:write" || p == "bank:demo" || p == "admin" {
			return true
		}
	}
	return false
}
