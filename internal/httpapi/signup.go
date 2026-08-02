package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kalke/kalke-auth/internal/mail"
	"github.com/kalke/kalke-auth/internal/security"
	"github.com/kalke/kalke-auth/internal/signup"
)

func (s *Server) signupStart(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.SignupEnabled {
		http.NotFound(w, r)
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "signup:"+ip, s.cfg.SignupRatePerMinute, time.Minute) ||
		!s.allowRate(r.Context(), "signup-hour:"+ip, s.cfg.SignupRatePerHour, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = mail.NormalizeEmail(body.Email)
	if body.Name == "" || body.Email == "" || body.Password == "" {
		writeErr(w, http.StatusBadRequest, "name, email and password required")
		return
	}
	if utf8.RuneCountInString(body.Name) < 2 || utf8.RuneCountInString(body.Name) > 80 {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	if !strings.Contains(body.Email, "@") {
		writeErr(w, http.StatusBadRequest, "invalid email")
		return
	}
	if msg := security.PasswordStrengthError(body.Password); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	// Owner/admin accounts are provisioned only in Keycloak.
	if s.isAdminEmail(body.Email) {
		writeErr(w, http.StatusUnauthorized, "invalid signup")
		return
	}

	pending, otp, err := s.pending.NewPending(body.Name, body.Email, body.Password)
	if err != nil {
		s.log.Error("signup pending", "err", err)
		writeErr(w, http.StatusInternalServerError, "signup failed")
		return
	}
	if err := s.pending.Put(r.Context(), pending); err != nil {
		s.log.Error("signup redis", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "signup unavailable")
		return
	}
	if err := s.sendSignupOTP(r.Context(), body.Email, otp); err != nil {
		s.log.Error("signup mail", "err", err)
		_ = s.pending.Delete(r.Context(), body.Email)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"status":               "pending_verification",
		"email":                body.Email,
		"resend_after_seconds": int(signup.ResendCooldown.Seconds()),
		"expires_in_seconds":   int(signup.OTPTTL.Seconds()),
	})
}

func (s *Server) signupVerify(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.SignupEnabled {
		http.NotFound(w, r)
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "signup-verify:"+ip, s.cfg.SignupRatePerMinute*2, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Email = mail.NormalizeEmail(body.Email)
	body.Code = strings.TrimSpace(body.Code)
	if body.Email == "" || body.Code == "" {
		writeErr(w, http.StatusBadRequest, "email and code required")
		return
	}

	pending, err := s.pending.Get(r.Context(), body.Email)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if err := s.pending.VerifyOTP(&pending, body.Code); err != nil {
		_ = s.pending.Put(r.Context(), pending) // persist attempt counter
		switch err {
		case signup.ErrTooManyTries, signup.ErrOTPExpired:
			_ = s.pending.Delete(r.Context(), body.Email)
			writeErr(w, http.StatusUnauthorized, "invalid code")
		default:
			writeErr(w, http.StatusUnauthorized, "invalid code")
		}
		return
	}

	password, err := s.pending.Password(pending)
	if err != nil {
		s.log.Error("signup unseal", "err", err)
		writeErr(w, http.StatusInternalServerError, "signup failed")
		return
	}
	if _, err := s.admin.CreateUser(r.Context(), pending.Name, pending.Email, password); err != nil {
		if strings.Contains(err.Error(), "user exists") {
			_ = s.pending.Delete(r.Context(), body.Email)
			writeErr(w, http.StatusConflict, "user exists")
			return
		}
		s.log.Error("signup create user", "err", err)
		writeErr(w, http.StatusBadGateway, "signup failed")
		return
	}
	_ = s.pending.Delete(r.Context(), body.Email)

	user, err := s.kc.PasswordLogin(r.Context(), pending.Email, password)
	if err != nil {
		s.log.Error("signup password login", "err", err, "email", pending.Email)
		writeJSON(w, http.StatusCreated, map[string]any{
			"ok":    true,
			"email": pending.Email,
		})
		return
	}
	if err := s.issueSession(w, r, user, pending.Email); err != nil {
		s.log.Error("signup session", "err", err)
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "email": pending.Email})
		return
	}
}

func (s *Server) signupResend(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.SignupEnabled {
		http.NotFound(w, r)
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "signup-resend:"+ip, s.cfg.SignupRatePerMinute, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Email = mail.NormalizeEmail(body.Email)
	if body.Email == "" {
		writeErr(w, http.StatusBadRequest, "email required")
		return
	}

	pending, err := s.pending.Get(r.Context(), body.Email)
	if err != nil {
		// Don't leak whether a pending signup exists.
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                   true,
			"resend_after_seconds": int(signup.ResendCooldown.Seconds()),
		})
		return
	}
	if wait, ok := s.pending.ResendAllowed(pending); !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":                "resend too soon",
			"resend_after_seconds": signup.SecondsUntil(time.Now().Add(wait)),
		})
		return
	}
	pending, otp, err := s.pending.RotateOTP(pending)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "resend failed")
		return
	}
	if err := s.pending.Put(r.Context(), pending); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "resend unavailable")
		return
	}
	if err := s.sendSignupOTP(r.Context(), body.Email, otp); err != nil {
		s.log.Error("signup resend mail", "err", err)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"resend_after_seconds": int(signup.ResendCooldown.Seconds()),
	})
}

func (s *Server) sendSignupOTP(ctx context.Context, email, otp string) error {
	subject, text, html := mail.SignupOTPEmail("kalke", otp)
	return s.mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: subject,
		Text:    text,
		HTML:    html,
	})
}

func jsonDecode(r *http.Request, dst any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(dst)
}
