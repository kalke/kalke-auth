package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kalke/kalke-auth/internal/keycloak"
	"github.com/kalke/kalke-auth/internal/security"
)

const oauthStateTTL = 10 * time.Minute

var oauthProviders = map[string]string{
	"google": "google",
	"github": "github",
	// "apple":  "apple",
}

type oauthPending struct {
	Provider     string `json:"provider"`
	CodeVerifier string `json:"code_verifier"`
	ReturnTo     string `json:"return_to"`
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "oauth-start:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	idpHint, ok := oauthProviders[provider]
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown provider")
		return
	}

	verifier, err := security.RandomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "oauth failed")
		return
	}
	state, err := security.RandomToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "oauth failed")
		return
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	pending := oauthPending{
		Provider:     provider,
		CodeVerifier: verifier,
		ReturnTo:     s.safeReturnURL(r.URL.Query().Get("return_to")),
	}
	if err := s.putOAuthPending(r.Context(), state, pending); err != nil {
		s.log.Error("oauth redis", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "oauth unavailable")
		return
	}

	authURL := s.kc.AuthorizationURL(s.cfg.OAuthRedirectURI, state, challenge, idpHint)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "oauth-callback:"+ip, s.cfg.LoginRatePerMinute*2, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	if errMsg := strings.TrimSpace(r.URL.Query().Get("error")); errMsg != "" {
		s.log.Info("oauth provider error", "error", errMsg, "desc", r.URL.Query().Get("error_description"))
		http.Redirect(w, r, s.oauthFailureURL(""), http.StatusFound)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		http.Redirect(w, r, s.oauthFailureURL(""), http.StatusFound)
		return
	}

	pending, err := s.takeOAuthPending(r.Context(), state)
	if err != nil {
		http.Redirect(w, r, s.oauthFailureURL(""), http.StatusFound)
		return
	}

	user, err := s.kc.ExchangeCode(r.Context(), code, s.cfg.OAuthRedirectURI, pending.CodeVerifier)
	if err != nil {
		s.log.Error("oauth exchange", "err", err, "provider", pending.Provider)
		http.Redirect(w, r, s.oauthFailureURL(pending.ReturnTo), http.StatusFound)
		return
	}
	s.syncOAuthProfile(r.Context(), user)
	if _, err := s.createSession(w, r, user, user.Email); err != nil {
		s.log.Error("oauth session", "err", err)
		http.Redirect(w, r, s.oauthFailureURL(pending.ReturnTo), http.StatusFound)
		return
	}
	http.Redirect(w, r, s.appendQuery(pending.ReturnTo, "oauth", "ok"), http.StatusFound)
}

func (s *Server) syncOAuthProfile(ctx context.Context, user keycloak.UserInfo) {
	if user.Subject == "" {
		return
	}
	first := strings.TrimSpace(user.GivenName)
	last := strings.TrimSpace(user.FamilyName)
	if first == "" && last == "" {
		first, last = keycloak.SplitDisplayName(user.DisplayName())
	}
	if first == "" && last == "" {
		return
	}
	if err := s.admin.UpdateUserNames(ctx, user.Subject, first, last); err != nil {
		s.log.Warn("oauth name sync", "err", err, "sub", user.Subject)
	}
}

func (s *Server) putOAuthPending(ctx context.Context, state string, p oauthPending) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, "oauth:state:"+state, raw, oauthStateTTL).Err()
}

func (s *Server) takeOAuthPending(ctx context.Context, state string) (oauthPending, error) {
	key := "oauth:state:" + state
	raw, err := s.rdb.GetDel(ctx, key).Bytes()
	if err != nil {
		return oauthPending{}, err
	}
	var p oauthPending
	if err := json.Unmarshal(raw, &p); err != nil {
		return oauthPending{}, err
	}
	return p, nil
}

func (s *Server) safeReturnURL(raw string) string {
	def := s.cfg.OAuthSuccessURL
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return def
	}
	if u.Fragment != "" {
		u.Fragment = ""
	}
	for _, origin := range s.cfg.CORSOrigins {
		o, err := url.Parse(origin)
		if err != nil || o.Host == "" {
			continue
		}
		if strings.EqualFold(o.Host, u.Host) {
			return u.String()
		}
	}
	return def
}

func (s *Server) oauthFailureURL(returnTo string) string {
	base := returnTo
	if base == "" {
		base = s.cfg.OAuthSuccessURL
	}
	return s.appendQuery(base, "oauth", "error")
}

func (s *Server) appendQuery(raw, key, value string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}
