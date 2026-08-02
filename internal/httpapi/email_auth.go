package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/kalke/kalke-auth/internal/keycloak"
	"github.com/kalke/kalke-auth/internal/mail"
	"github.com/kalke/kalke-auth/internal/otp"
	"github.com/kalke/kalke-auth/internal/security"
)

func (s *Server) passwordlessStart(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "login-email:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
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
	if body.Email == "" || !strings.Contains(body.Email, "@") {
		writeErr(w, http.StatusBadRequest, "invalid email")
		return
	}

	// Always respond the same way to avoid account enumeration.
	okResp := map[string]any{
		"ok":                   true,
		"status":               "pending_verification",
		"email":                body.Email,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
		"expires_in_seconds":   int(otp.OTPTTL.Seconds()),
	}

	user, err := s.admin.FindUserByEmail(r.Context(), body.Email)
	if err != nil || !user.Enabled {
		writeJSON(w, http.StatusOK, okResp)
		return
	}

	challenge, code, err := s.loginOTP.New(body.Email)
	if err != nil {
		s.log.Error("passwordless pending", "err", err)
		writeErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	if err := s.loginOTP.Put(r.Context(), challenge); err != nil {
		s.log.Error("passwordless redis", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "login unavailable")
		return
	}
	if err := s.sendAuthOTP(r.Context(), mail.OTPPasswordless, body.Email, code); err != nil {
		s.log.Error("passwordless mail", "err", err)
		_ = s.loginOTP.Delete(r.Context(), body.Email)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, okResp)
}

func (s *Server) passwordlessVerify(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "login-email-verify:"+ip, s.cfg.LoginRatePerMinute*2, time.Minute) {
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

	challenge, err := s.loginOTP.Get(r.Context(), body.Email)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if err := s.loginOTP.Verify(&challenge, body.Code); err != nil {
		_ = s.loginOTP.Put(r.Context(), challenge)
		switch err {
		case otp.ErrTooManyTries, otp.ErrOTPExpired:
			_ = s.loginOTP.Delete(r.Context(), body.Email)
		}
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	_ = s.loginOTP.Delete(r.Context(), body.Email)

	user, err := s.admin.FindUserByEmail(r.Context(), body.Email)
	if err != nil || !user.Enabled {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	roles, err := s.admin.ListRealmRoleNames(r.Context(), user.ID)
	if err != nil {
		s.log.Error("passwordless roles", "err", err, "email", body.Email)
		writeErr(w, http.StatusBadGateway, "login failed")
		return
	}
	info := keycloak.UserInfo{
		Subject:     user.ID,
		Email:       user.Email,
		Permissions: roles,
	}
	if err := s.issueSession(w, r, info, body.Email); err != nil {
		s.log.Error("passwordless session", "err", err)
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
}

func (s *Server) passwordlessResend(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "login-email-resend:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
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

	okResp := map[string]any{
		"ok":                   true,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
	}
	challenge, err := s.loginOTP.Get(r.Context(), body.Email)
	if err != nil {
		writeJSON(w, http.StatusOK, okResp)
		return
	}
	if wait, ok := s.loginOTP.ResendAllowed(challenge); !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":                "resend too soon",
			"resend_after_seconds": otp.SecondsUntil(time.Now().Add(wait)),
		})
		return
	}
	challenge, code, err := s.loginOTP.Rotate(challenge)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "resend failed")
		return
	}
	if err := s.loginOTP.Put(r.Context(), challenge); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "resend unavailable")
		return
	}
	if err := s.sendAuthOTP(r.Context(), mail.OTPPasswordless, body.Email, code); err != nil {
		s.log.Error("passwordless resend mail", "err", err)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, okResp)
}

func (s *Server) forgotPasswordStart(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "forgot:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
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
	if body.Email == "" || !strings.Contains(body.Email, "@") {
		writeErr(w, http.StatusBadRequest, "invalid email")
		return
	}

	okResp := map[string]any{
		"ok":                   true,
		"status":               "pending_verification",
		"email":                body.Email,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
		"expires_in_seconds":   int(otp.OTPTTL.Seconds()),
	}

	user, err := s.admin.FindUserByEmail(r.Context(), body.Email)
	if err != nil || !user.Enabled {
		writeJSON(w, http.StatusOK, okResp)
		return
	}

	challenge, code, err := s.resetOTP.New(body.Email)
	if err != nil {
		s.log.Error("forgot pending", "err", err)
		writeErr(w, http.StatusInternalServerError, "reset failed")
		return
	}
	if err := s.resetOTP.Put(r.Context(), challenge); err != nil {
		s.log.Error("forgot redis", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "reset unavailable")
		return
	}
	if err := s.sendAuthOTP(r.Context(), mail.OTPReset, body.Email, code); err != nil {
		s.log.Error("forgot mail", "err", err)
		_ = s.resetOTP.Delete(r.Context(), body.Email)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, okResp)
}

func (s *Server) forgotPasswordVerify(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "forgot-verify:"+ip, s.cfg.LoginRatePerMinute*2, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	body.Email = mail.NormalizeEmail(body.Email)
	body.Code = strings.TrimSpace(body.Code)
	if body.Email == "" || body.Code == "" || body.NewPassword == "" {
		writeErr(w, http.StatusBadRequest, "email, code and new_password required")
		return
	}
	if msg := security.PasswordStrengthError(body.NewPassword); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}

	challenge, err := s.resetOTP.Get(r.Context(), body.Email)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if err := s.resetOTP.Verify(&challenge, body.Code); err != nil {
		_ = s.resetOTP.Put(r.Context(), challenge)
		switch err {
		case otp.ErrTooManyTries, otp.ErrOTPExpired:
			_ = s.resetOTP.Delete(r.Context(), body.Email)
		}
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}

	user, err := s.admin.FindUserByEmail(r.Context(), body.Email)
	if err != nil || !user.Enabled {
		_ = s.resetOTP.Delete(r.Context(), body.Email)
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if err := s.admin.SetPassword(r.Context(), user.ID, body.NewPassword); err != nil {
		s.log.Error("forgot set password", "err", err, "email", body.Email)
		writeErr(w, http.StatusBadGateway, "password update failed")
		return
	}
	_ = s.resetOTP.Delete(r.Context(), body.Email)

	info, err := s.kc.PasswordLogin(r.Context(), body.Email, body.NewPassword)
	if err != nil {
		s.log.Error("forgot password login", "err", err, "email", body.Email)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "email": body.Email})
		return
	}
	if err := s.issueSession(w, r, info, body.Email); err != nil {
		s.log.Error("forgot session", "err", err)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "email": body.Email})
		return
	}
}

func (s *Server) forgotPasswordResend(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "forgot-resend:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
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

	okResp := map[string]any{
		"ok":                   true,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
	}
	challenge, err := s.resetOTP.Get(r.Context(), body.Email)
	if err != nil {
		writeJSON(w, http.StatusOK, okResp)
		return
	}
	if wait, ok := s.resetOTP.ResendAllowed(challenge); !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":                "resend too soon",
			"resend_after_seconds": otp.SecondsUntil(time.Now().Add(wait)),
		})
		return
	}
	challenge, code, err := s.resetOTP.Rotate(challenge)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "resend failed")
		return
	}
	if err := s.resetOTP.Put(r.Context(), challenge); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "resend unavailable")
		return
	}
	if err := s.sendAuthOTP(r.Context(), mail.OTPReset, body.Email, code); err != nil {
		s.log.Error("forgot resend mail", "err", err)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, okResp)
}

func (s *Server) sendAuthOTP(ctx context.Context, kind mail.OTPKind, email, code string) error {
	subject, text, html := mail.OTPEmail(kind, "kalke", code)
	return s.mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: subject,
		Text:    text,
		HTML:    html,
	})
}
