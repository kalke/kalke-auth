package httpapi

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kalke/kalke-auth/internal/mail"
	"github.com/kalke/kalke-auth/internal/otp"
)

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "profile:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	var body struct {
		Name *string `json:"name"`
	}
	if err := jsonDecode(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == nil {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	name := strings.TrimSpace(*body.Name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 120 {
		writeErr(w, http.StatusBadRequest, "invalid name")
		return
	}
	if err := s.admin.UpdateUserName(r.Context(), p.UserSub, name); err != nil {
		s.log.Error("auth.profile.update",
			"outcome", "error",
			"error_code", "profile_update_failed",
			"sub", p.UserSub,
			"email", p.UserEmail,
			"err", err,
			"request_id", requestIDFrom(r.Context()),
		)
		writeErr(w, http.StatusBadGateway, "profile update failed")
		return
	}
	s.log.Info("auth.profile.update",
		"outcome", "ok",
		"sub", p.UserSub,
		"email", p.UserEmail,
		"request_id", requestIDFrom(r.Context()),
	)
	// Re-read so response matches what Keycloak stored (first + last).
	display := name
	if u, err := s.admin.GetUser(r.Context(), p.UserSub); err == nil {
		if n := u.DisplayName(); n != "" {
			display = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"name":        display,
		"email":       p.UserEmail,
		"permissions": s.effectivePermissions(p.UserEmail, p.Permissions),
	})
}

func (s *Server) emailChangeStart(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "email-change:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
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
	newEmail := mail.NormalizeEmail(body.Email)
	if newEmail == "" || !strings.Contains(newEmail, "@") {
		writeErr(w, http.StatusBadRequest, "invalid email")
		return
	}
	if newEmail == p.UserEmail {
		writeErr(w, http.StatusBadRequest, "email unchanged")
		return
	}
	if existing, err := s.admin.FindUserByEmail(r.Context(), newEmail); err == nil && existing.ID != "" && existing.ID != p.UserSub {
		writeErr(w, http.StatusConflict, "email taken")
		return
	}

	challenge, code, err := s.emailOTP.New(newEmail)
	if err != nil {
		s.log.Error("email change pending", "err", err)
		writeErr(w, http.StatusInternalServerError, "email change failed")
		return
	}
	challenge.UserSub = p.UserSub
	if err := s.emailOTP.Put(r.Context(), challenge); err != nil {
		s.log.Error("email change redis", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "email change unavailable")
		return
	}
	if err := s.sendAuthOTP(r.Context(), mail.OTPEmailChange, newEmail, code); err != nil {
		s.log.Error("email change mail", "err", err)
		_ = s.emailOTP.Delete(r.Context(), newEmail)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"status":               "pending_verification",
		"email":                newEmail,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
		"expires_in_seconds":   int(otp.OTPTTL.Seconds()),
	})
}

func (s *Server) emailChangeVerify(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "email-change-verify:"+ip, s.cfg.LoginRatePerMinute*2, time.Minute) {
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
	newEmail := mail.NormalizeEmail(body.Email)
	code := strings.TrimSpace(body.Code)
	if newEmail == "" || code == "" {
		writeErr(w, http.StatusBadRequest, "email and code required")
		return
	}

	challenge, err := s.emailOTP.Get(r.Context(), newEmail)
	if err != nil || challenge.UserSub != p.UserSub {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if err := s.emailOTP.Verify(&challenge, code); err != nil {
		_ = s.emailOTP.Put(r.Context(), challenge)
		switch err {
		case otp.ErrTooManyTries, otp.ErrOTPExpired:
			_ = s.emailOTP.Delete(r.Context(), newEmail)
		}
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	_ = s.emailOTP.Delete(r.Context(), newEmail)

	if existing, err := s.admin.FindUserByEmail(r.Context(), newEmail); err == nil && existing.ID != "" && existing.ID != p.UserSub {
		writeErr(w, http.StatusConflict, "email taken")
		return
	}
	if err := s.admin.UpdateUserEmail(r.Context(), p.UserSub, newEmail); err != nil {
		s.log.Error("email change update", "err", err, "sub", p.UserSub)
		writeErr(w, http.StatusBadGateway, "email update failed")
		return
	}
	if err := s.store.UpdateSessionEmail(r.Context(), p.SessionID, newEmail); err != nil {
		s.log.Error("email change session", "err", err, "sub", p.UserSub)
		writeErr(w, http.StatusInternalServerError, "session update failed")
		return
	}

	name := ""
	if u, err := s.admin.GetUser(r.Context(), p.UserSub); err == nil {
		name = u.DisplayName()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"name":        name,
		"email":       newEmail,
		"permissions": s.effectivePermissions(newEmail, p.Permissions),
	})
}

func (s *Server) emailChangeResend(w http.ResponseWriter, r *http.Request) {
	p, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	ip := clientIP(r)
	if !s.allowRate(r.Context(), "email-change-resend:"+ip, s.cfg.LoginRatePerMinute, time.Minute) {
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
	newEmail := mail.NormalizeEmail(body.Email)
	if newEmail == "" {
		writeErr(w, http.StatusBadRequest, "invalid email")
		return
	}
	challenge, err := s.emailOTP.Get(r.Context(), newEmail)
	if err != nil || challenge.UserSub != p.UserSub {
		writeErr(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if wait, ok := s.emailOTP.ResendAllowed(challenge); !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok":                   false,
			"resend_after_seconds": otp.SecondsUntil(time.Now().UTC().Add(wait)),
		})
		return
	}
	challenge, code, err := s.emailOTP.Rotate(challenge)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "email change failed")
		return
	}
	challenge.UserSub = p.UserSub
	if err := s.emailOTP.Put(r.Context(), challenge); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "email change unavailable")
		return
	}
	if err := s.sendAuthOTP(r.Context(), mail.OTPEmailChange, newEmail, code); err != nil {
		s.log.Error("email change resend mail", "err", err)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
	})
}
