package httpapi

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kalke/kalke-auth/internal/config"
	"github.com/kalke/kalke-auth/internal/keycloak"
	"github.com/kalke/kalke-auth/internal/mail"
	"github.com/kalke/kalke-auth/internal/otp"
	"github.com/kalke/kalke-auth/internal/security"
	"github.com/kalke/kalke-auth/internal/signup"
	"github.com/kalke/kalke-auth/internal/store"
)

const sessionCookie = "kalke_session"

type Server struct {
	cfg       config.Config
	store     *store.Store
	kc        *keycloak.Client
	admin     *keycloak.AdminClient
	rdb       *redis.Client
	pending   *signup.Store
	loginOTP  *otp.Store
	resetOTP  *otp.Store
	mailer    mail.Mailer
	proxy     *httputil.ReverseProxy
	pdeHTTP   *http.Client
	ebankHTTP *http.Client
	m2m       m2mCache
	ebankM2M  m2mCache
	log       *slog.Logger
}

type sessionPrincipal struct {
	SessionID   uuid.UUID
	UserSub     string
	UserEmail   string
	Permissions []string
}

func New(cfg config.Config, st *store.Store, kc *keycloak.Client, admin *keycloak.AdminClient, rdb *redis.Client, mailer mail.Mailer, log *slog.Logger) *Server {
	target, _ := url.Parse(cfg.KCInternalURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("keycloak proxy", "err", err)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	if mailer == nil {
		mailer = mail.LogMailer{Log: log}
	}
	return &Server{
		cfg:       cfg,
		store:     st,
		kc:        kc,
		admin:     admin,
		rdb:       rdb,
		pending:   signup.NewStore(rdb, cfg.TokenPepper),
		loginOTP:  otp.NewStore(rdb, cfg.TokenPepper, "login:otp:"),
		resetOTP:  otp.NewStore(rdb, cfg.TokenPepper, "reset:otp:"),
		mailer:    mailer,
		proxy:     proxy,
		pdeHTTP:   &http.Client{Timeout: 180 * time.Second},
		ebankHTTP: &http.Client{Timeout: 60 * time.Second},
		log:       log,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /v1/auth/login", s.login)
	mux.HandleFunc("GET /v1/auth/oauth/{provider}", s.oauthStart)
	mux.HandleFunc("GET /v1/auth/callback", s.oauthCallback)
	mux.HandleFunc("POST /v1/auth/login/email", s.passwordlessStart)
	mux.HandleFunc("POST /v1/auth/login/email/verify", s.passwordlessVerify)
	mux.HandleFunc("POST /v1/auth/login/email/resend", s.passwordlessResend)
	mux.HandleFunc("POST /v1/auth/signup", s.signupStart)
	mux.HandleFunc("POST /v1/auth/signup/verify", s.signupVerify)
	mux.HandleFunc("POST /v1/auth/signup/resend", s.signupResend)
	mux.HandleFunc("POST /v1/auth/logout", s.logout)
	mux.HandleFunc("GET /v1/auth/me", s.me)
	mux.HandleFunc("POST /v1/auth/password", s.changePassword)
	mux.HandleFunc("POST /v1/auth/password/forgot", s.forgotPasswordStart)
	mux.HandleFunc("POST /v1/auth/password/forgot/verify", s.forgotPasswordVerify)
	mux.HandleFunc("POST /v1/auth/password/forgot/resend", s.forgotPasswordResend)
	mux.HandleFunc("POST /v1/tokens", s.createToken)
	mux.HandleFunc("GET /v1/tokens", s.listTokens)
	mux.HandleFunc("DELETE /v1/tokens/{id}", s.revokeToken)
	mux.HandleFunc("POST /v1/introspect", s.introspect)
	mux.HandleFunc("POST /v1/extract", s.proxyExtract)
	mux.HandleFunc("GET /v1/extractions", s.proxyExtractions)
	mux.HandleFunc("GET /v1/extractions/{id}", s.proxyExtractions)
	mux.HandleFunc("GET /v1/bank/meta", s.bankProxy("/v1/demo/meta"))
	mux.HandleFunc("POST /v1/bank/bootstrap", s.bankProxy("/v1/demo/bootstrap"))
	mux.HandleFunc("GET /v1/bank/account", s.bankProxy("/v1/me/account"))
	mux.HandleFunc("GET /v1/bank/transactions", s.bankProxy("/v1/me/transactions"))
	mux.HandleFunc("POST /v1/bank/transfer", s.bankProxy("/v1/me/transfer"))
	mux.HandleFunc("POST /v1/bank/withdraw", s.bankProxy("/v1/me/withdraw"))
	mux.HandleFunc("GET /v1/bank/onboarding", s.bankProxy("/v1/onboarding"))
	mux.HandleFunc("POST /v1/bank/onboarding/start", s.bankProxy("/v1/onboarding/start"))
	mux.HandleFunc("POST /v1/bank/onboarding/consent", s.bankProxy("/v1/onboarding/consent"))
	mux.HandleFunc("POST /v1/bank/onboarding/skip", s.bankProxy("/v1/onboarding/skip"))
	mux.HandleFunc("POST /v1/bank/onboarding/documents", s.bankProxy("/v1/onboarding/documents"))
	mux.HandleFunc("POST /v1/bank/onboarding/complete", s.bankProxy("/v1/onboarding/complete"))
	mux.HandleFunc("/", s.oidcProxy)
	return s.cors(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "kalke-auth"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "login:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || body.Password == "" {
		writeErr(w, http.StatusBadRequest, "email and password required")
		return
	}
	user, err := s.kc.PasswordLogin(r.Context(), body.Email, body.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := s.issueSession(w, r, user, body.Email); err != nil {
		s.log.Error("create session", "err", err)
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, user keycloak.UserInfo, fallbackEmail string) error {
	sess, err := s.createSession(w, r, user, fallbackEmail)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":       sess.UserEmail,
		"permissions": sess.Permissions,
	})
	return nil
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, user keycloak.UserInfo, fallbackEmail string) (store.Session, error) {
	raw, err := security.RandomToken()
	if err != nil {
		return store.Session{}, err
	}
	hash, err := security.HashSecret(s.cfg.TokenPepper, raw)
	if err != nil {
		return store.Session{}, err
	}
	email := user.Email
	if email == "" {
		email = fallbackEmail
	}
	id := uuid.New()
	sess := store.Session{
		ID:          id,
		UserSub:     user.Subject,
		UserEmail:   email,
		Permissions: s.effectivePermissions(email, user.Permissions),
		TokenHash:   hash,
		ExpiresAt:   time.Now().UTC().Add(s.cfg.SessionTTL),
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		return store.Session{}, err
	}
	s.setSessionCookie(w, id.String()+"."+raw, int(s.cfg.SessionTTL.Seconds()))
	return sess, nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "password:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if msg := passwordChangeValidationError(body.CurrentPassword, body.NewPassword); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if _, err := s.kc.PasswordLogin(r.Context(), p.UserEmail, body.CurrentPassword); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := s.admin.SetPassword(r.Context(), p.UserSub, body.NewPassword); err != nil {
		s.log.Error("change password", "err", err, "sub", p.UserSub)
		writeErr(w, http.StatusBadGateway, "password update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func passwordChangeValidationError(current, next string) string {
	if current == "" || next == "" {
		return "current and new password required"
	}
	if msg := security.PasswordStrengthError(next); msg != "" {
		return msg
	}
	if current == next {
		return "new password must differ"
	}
	return ""
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if p, err := s.principalFromRequest(r); err == nil {
		_ = s.store.DeleteSession(r.Context(), p.SessionID)
	}
	s.setSessionCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Domain:   s.cfg.CookieDomain,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: cookieSameSite(s.cfg.CookieSameSite),
		MaxAge:   maxAge,
	})
}

func cookieSameSite(mode string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "lax":
		return http.SameSiteLaxMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteNoneMode
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":       p.UserEmail,
		"permissions": s.effectivePermissions(p.UserEmail, p.Permissions),
	})
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		body.Name = "default"
	}
	plain, prefix, hash, err := security.NewPAT(s.cfg.TokenPepper)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	id := uuid.New()
	tok := store.APIToken{
		ID:          id,
		UserSub:     p.UserSub,
		UserEmail:   p.UserEmail,
		Name:        body.Name,
		TokenPrefix: prefix,
		TokenHash:   hash,
		Permissions: s.effectivePermissions(p.UserEmail, p.Permissions),
	}
	if err := s.store.CreateAPIToken(r.Context(), tok); err != nil {
		s.log.Error("create token", "err", err)
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     id.String(),
		"name":   tok.Name,
		"prefix": prefix,
		"token":  plain, // shown once
	})
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := s.store.ListAPITokens(r.Context(), p.UserSub)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, t := range items {
		m := map[string]any{
			"id":         t.ID.String(),
			"name":       t.Name,
			"prefix":     t.TokenPrefix,
			"created_at": t.CreatedAt.UTC().Format(time.RFC3339),
		}
		if t.LastUsedAt != nil {
			m["last_used_at"] = t.LastUsedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.store.RevokeAPIToken(r.Context(), id, p.UserSub); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "revoke error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) introspect(w http.ResponseWriter, r *http.Request) {
	if !secureCompare(r.Header.Get("X-Kalke-Introspect-Key"), s.cfg.IntrospectSecret) {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	token := strings.TrimSpace(body.Token)
	if !security.IsPAT(token) {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	prefix, err := security.PATPrefixFromToken(token)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	row, err := s.store.GetAPITokenByPrefix(r.Context(), prefix)
	if err != nil || !security.CheckSecret(s.cfg.TokenPepper, token, row.TokenHash) {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	_ = s.store.TouchAPIToken(r.Context(), row.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"active":      true,
		"sub":         row.UserSub,
		"email":       row.UserEmail,
		"permissions": s.effectivePermissions(row.UserEmail, row.Permissions),
	})
}

func (s *Server) oidcProxy(w http.ResponseWriter, r *http.Request) {
	if !isPublicOIDCPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	s.proxy.ServeHTTP(w, r)
}

func isPublicOIDCPath(path string) bool {
	// Browser OAuth (Google IdP) needs auth + broker (+ optional login UI).
	// Token/admin stay internal — the BFF exchanges codes via KC_INTERNAL_URL.
	p := strings.ToLower(path)
	switch {
	case p == "/realms/kalke/.well-known/openid-configuration":
		return true
	case strings.HasPrefix(p, "/realms/kalke/protocol/openid-connect/certs"):
		return true
	case p == "/realms/kalke/protocol/openid-connect/auth":
		return true
	case strings.HasPrefix(p, "/realms/kalke/broker/"):
		return true
	case strings.HasPrefix(p, "/realms/kalke/login-actions/"):
		return true
	case strings.HasPrefix(p, "/resources/"):
		return true
	default:
		return false
	}
}

func (s *Server) principalFromRequest(r *http.Request) (sessionPrincipal, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return sessionPrincipal{}, errors.New("no session")
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return sessionPrincipal{}, errors.New("bad session")
	}
	id, err := uuid.Parse(parts[0])
	if err != nil {
		return sessionPrincipal{}, err
	}
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		return sessionPrincipal{}, err
	}
	if !security.CheckSecret(s.cfg.TokenPepper, parts[1], sess.TokenHash) {
		return sessionPrincipal{}, errors.New("bad session")
	}
	return sessionPrincipal{
		SessionID:   sess.ID,
		UserSub:     sess.UserSub,
		UserEmail:   sess.UserEmail,
		Permissions: sess.Permissions,
	}, nil
}

func (s *Server) allowRate(ctx context.Context, key string, limit int, window time.Duration) bool {
	// Fail closed: without Redis we refuse auth endpoints rather than unbounded traffic.
	if s.rdb == nil || key == "" || limit <= 0 {
		return false
	}
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		s.log.Warn("redis rate limit", "err", err)
		return false
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, window).Err()
	}
	return n <= int64(limit)
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, o := range s.cfg.CORSOrigins {
		allowed[o] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Kalke-Introspect-Key")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func NewRedis(cfg config.Config) *redis.Client {
	opts := &redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	}
	if cfg.RedisTLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opts)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("CF-Connecting-IP"); xff != "" {
		return xff
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func secureCompare(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
