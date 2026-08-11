package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kalke/kalke-auth/internal/mail"
	"github.com/kalke/kalke-auth/internal/otp"
)

type transferChallengeBody struct {
	Amount                 string `json:"amount"`
	Memo                   string `json:"memo"`
	SourceAccountID        string `json:"source_account_id"`
	DestinationAccount     string `json:"destination_account"`
	DestinationAccountID   string `json:"destination_account_id"`
	DestinationDocument    string `json:"destination_document"`
}

type transferConfirmBody struct {
	Code string `json:"code"`
}

func (s *Server) transferChallengeStart(w http.ResponseWriter, r *http.Request) {
	prin, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms := s.effectivePermissions(prin.UserEmail, prin.Permissions)
	if !hasBankPermission(perms) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if !s.allowRate(r.Context(), "xfer-challenge:"+prin.UserSub, 8, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	if strings.TrimSpace(prin.UserEmail) == "" {
		writeErr(w, http.StatusBadRequest, "account email required for transfer confirmation")
		return
	}

	var body transferChallengeBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	amount := strings.TrimSpace(body.Amount)
	if amount == "" {
		writeErr(w, http.StatusBadRequest, "amount required")
		return
	}
	destAccount := strings.TrimSpace(body.DestinationAccount)
	destAccountID := strings.TrimSpace(body.DestinationAccountID)
	destDocument := strings.TrimSpace(body.DestinationDocument)
	n := 0
	if destAccount != "" {
		n++
	}
	if destAccountID != "" {
		n++
	}
	if destDocument != "" {
		n++
	}
	if n != 1 {
		writeErr(w, http.StatusBadRequest, "provide exactly one destination")
		return
	}

	holder, display := "", ""
	if destAccountID == "" {
		resolved, status, errMsg := s.bankResolveRecipient(r.Context(), prin, destAccount, destDocument)
		if status != http.StatusOK {
			writeErr(w, status, errMsg)
			return
		}
		holder = resolved.HolderName
		display = resolved.AccountDisplay
		if display == "" {
			display = destAccount
		}
	} else {
		display = destAccountID
	}

	idemKey := uuid.NewString()
	pending, code, err := s.xferOTP.New(
		prin.UserSub,
		prin.UserEmail,
		amount,
		destAccount,
		destAccountID,
		destDocument,
		holder,
		display,
		strings.TrimSpace(body.Memo),
		strings.TrimSpace(body.SourceAccountID),
		idemKey,
	)
	if err != nil {
		s.log.Error("transfer challenge create", "err", err)
		writeErr(w, http.StatusInternalServerError, "transfer challenge failed")
		return
	}
	if err := s.xferOTP.Put(r.Context(), pending); err != nil {
		s.log.Error("transfer challenge redis", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "transfer challenge unavailable")
		return
	}
	if err := s.sendTransferOTP(r.Context(), pending.Email, code, pending); err != nil {
		s.log.Error("transfer challenge mail", "err", err)
		_ = s.xferOTP.Delete(r.Context(), prin.UserSub)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "pending_verification",
		"email_masked":         mail.MaskEmail(pending.Email),
		"amount":               pending.Amount,
		"destination_display":  pending.DestinationDisplay,
		"destination_holder":   pending.DestinationHolder,
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
		"expires_in_seconds":   int(otp.OTPTTL.Seconds()),
	})
}

func (s *Server) transferChallengeResend(w http.ResponseWriter, r *http.Request) {
	prin, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms := s.effectivePermissions(prin.UserEmail, prin.Permissions)
	if !hasBankPermission(perms) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if !s.allowRate(r.Context(), "xfer-resend:"+prin.UserSub, 6, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	pending, err := s.xferOTP.Get(r.Context(), prin.UserSub)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no pending transfer")
		return
	}
	if wait, ok := s.xferOTP.ResendAllowed(pending); !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":                "resend cooldown",
			"resend_after_seconds": otp.SecondsUntil(time.Now().UTC().Add(wait)),
		})
		return
	}
	pending, code, err := s.xferOTP.Rotate(pending)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "transfer challenge failed")
		return
	}
	if err := s.xferOTP.Put(r.Context(), pending); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "transfer challenge unavailable")
		return
	}
	if err := s.sendTransferOTP(r.Context(), pending.Email, code, pending); err != nil {
		s.log.Error("transfer challenge resend mail", "err", err)
		writeErr(w, http.StatusBadGateway, "email send failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":               "pending_verification",
		"email_masked":         mail.MaskEmail(pending.Email),
		"resend_after_seconds": int(otp.ResendCooldown.Seconds()),
		"expires_in_seconds":   int(otp.OTPTTL.Seconds()),
	})
}

func (s *Server) transferConfirm(w http.ResponseWriter, r *http.Request) {
	prin, err := s.principalFromRequest(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	perms := s.effectivePermissions(prin.UserEmail, prin.Permissions)
	if !hasBankPermission(perms) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	if !s.allowRate(r.Context(), "xfer-confirm:"+prin.UserSub, 20, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	var body transferConfirmBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		writeErr(w, http.StatusBadRequest, "code required")
		return
	}

	pending, err := s.xferOTP.Get(r.Context(), prin.UserSub)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no pending transfer")
		return
	}
	if err := s.xferOTP.Verify(&pending, code); err != nil {
		_ = s.xferOTP.Put(r.Context(), pending)
		switch err {
		case otp.ErrTooManyTries, otp.ErrOTPExpired:
			_ = s.xferOTP.Delete(r.Context(), prin.UserSub)
			writeErr(w, http.StatusUnauthorized, err.Error())
		default:
			writeErr(w, http.StatusUnauthorized, "invalid code")
		}
		return
	}
	_ = s.xferOTP.Delete(r.Context(), prin.UserSub)

	payload := map[string]any{
		"amount": pending.Amount,
	}
	if pending.Memo != "" {
		payload["memo"] = pending.Memo
	}
	if pending.SourceAccountID != "" {
		payload["source_account_id"] = pending.SourceAccountID
	}
	switch {
	case pending.DestinationAccountID != "":
		payload["destination_account_id"] = pending.DestinationAccountID
	case pending.DestinationAccount != "":
		payload["destination_account"] = pending.DestinationAccount
	case pending.DestinationDocument != "":
		payload["destination_document"] = pending.DestinationDocument
	}

	status, respBody, headers, err := s.bankJSON(
		r.Context(),
		prin,
		http.MethodPost,
		"/v1/me/transfer",
		pending.IdempotencyKey,
		payload,
	)
	if err != nil {
		s.log.Error("transfer confirm upstream", "err", err, "sub", prin.UserSub)
		writeErr(w, http.StatusBadGateway, "upstream unavailable")
		return
	}
	if status == http.StatusOK {
		s.sendTransferReceipts(r.Context(), prin, pending, respBody)
	}
	for _, h := range []string{
		"Content-Type", "Cache-Control", "Retry-After",
		"X-RateLimit-Limit", "X-RateLimit-Remaining", "X-Request-ID",
	} {
		if v := headers.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(respBody)
}

type bankTransferResult struct {
	Origin struct {
		ID            string  `json:"id"`
		DisplayNumber string  `json:"display_number"`
		Balance       string  `json:"balance"`
		HolderName    *string `json:"holder_name"`
	} `json:"origin"`
	Destination struct {
		ID            string  `json:"id"`
		DisplayNumber string  `json:"display_number"`
		Balance       string  `json:"balance"`
		HolderName    *string `json:"holder_name"`
		HolderEmail   *string `json:"holder_email"`
	} `json:"destination"`
	Amount   string  `json:"amount"`
	Memo     *string `json:"memo"`
	Currency string  `json:"currency"`
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func (s *Server) sendTransferReceipts(
	ctx context.Context,
	prin sessionPrincipal,
	pending otp.TransferPending,
	respBody []byte,
) {
	var result bankTransferResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		s.log.Warn("transfer receipt parse", "err", err)
		return
	}
	when := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	currency := strings.TrimSpace(result.Currency)
	if currency == "" {
		currency = "USD"
	}
	amount := strings.TrimSpace(result.Amount)
	if amount == "" {
		amount = pending.Amount
	}
	memo := derefStr(result.Memo)
	if memo == "" {
		memo = pending.Memo
	}
	destDisplay := strings.TrimSpace(result.Destination.DisplayNumber)
	if destDisplay == "" {
		destDisplay = pending.DestinationDisplay
	}
	destHolder := derefStr(result.Destination.HolderName)
	if destHolder == "" {
		destHolder = pending.DestinationHolder
	}

	if email := strings.TrimSpace(prin.UserEmail); email != "" {
		subject, text, html := mail.TransferSentEmail("kalke", mail.TransferReceiptDetails{
			Amount:       amount,
			Currency:     currency,
			Counterparty: destDisplay,
			Holder:       destHolder,
			Memo:         memo,
			Balance:      strings.TrimSpace(result.Origin.Balance),
			When:         when,
		})
		if err := s.mailer.Send(ctx, mail.Message{
			To: email, Subject: subject, Text: text, HTML: html,
		}); err != nil {
			s.log.Error("transfer sent mail", "err", err, "to", email)
		}
	}

	recipientEmail := strings.ToLower(derefStr(result.Destination.HolderEmail))
	senderEmail := strings.ToLower(strings.TrimSpace(prin.UserEmail))
	if recipientEmail == "" || recipientEmail == senderEmail {
		if recipientEmail == "" {
			s.log.Info("transfer received mail skipped", "reason", "no recipient email")
		}
		return
	}
	originDisplay := strings.TrimSpace(result.Origin.DisplayNumber)
	originHolder := derefStr(result.Origin.HolderName)
	subject, text, html := mail.TransferReceivedEmail("kalke", mail.TransferReceiptDetails{
		Amount:       amount,
		Currency:     currency,
		Counterparty: originDisplay,
		Holder:       originHolder,
		Memo:         memo,
		Balance:      strings.TrimSpace(result.Destination.Balance),
		When:         when,
	})
	if err := s.mailer.Send(ctx, mail.Message{
		To: recipientEmail, Subject: subject, Text: text, HTML: html,
	}); err != nil {
		s.log.Error("transfer received mail", "err", err, "to", recipientEmail)
	}
}

type bankResolveResult struct {
	AccountID      string `json:"account_id"`
	AccountDisplay string `json:"account_display"`
	HolderName     string `json:"holder_name"`
}

func (s *Server) bankResolveRecipient(
	ctx context.Context,
	prin sessionPrincipal,
	account, document string,
) (bankResolveResult, int, string) {
	payload := map[string]any{}
	if account != "" {
		payload["account"] = account
	}
	if document != "" {
		payload["document"] = document
	}
	status, body, _, err := s.bankJSON(ctx, prin, http.MethodPost, "/v1/me/transfers/resolve", "", payload)
	if err != nil {
		return bankResolveResult{}, http.StatusBadGateway, "upstream unavailable"
	}
	if status >= 400 {
		var errBody struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(body, &errBody)
		msg := errBody.Message
		if msg == "" {
			msg = errBody.Error
		}
		if msg == "" {
			msg = "could not resolve recipient"
		}
		return bankResolveResult{}, status, msg
	}
	var out bankResolveResult
	if err := json.Unmarshal(body, &out); err != nil {
		return bankResolveResult{}, http.StatusBadGateway, "invalid upstream response"
	}
	return out, http.StatusOK, ""
}

func (s *Server) bankJSON(
	ctx context.Context,
	prin sessionPrincipal,
	method, path, idemKey string,
	payload any,
) (int, []byte, http.Header, error) {
	up := s.ebankUpstream()
	if up.baseURL == "" {
		return 0, nil, nil, errBankUnavailable
	}
	bearer, err := up.token(ctx)
	if err != nil {
		return 0, nil, nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, up.baseURL+path, bytes.NewReader(raw)) // #nosec G704
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-Kalke-User-Email", prin.UserEmail)
	req.Header.Set("X-Kalke-User-Sub", prin.UserSub)
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	if up.forwardSecret != "" {
		req.Header.Set("X-Kalke-Forward-Secret", up.forwardSecret)
	}
	resp, err := up.client.Do(req) // #nosec G704
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, body, resp.Header.Clone(), nil
}

func (s *Server) sendTransferOTP(ctx context.Context, email, code string, pending otp.TransferPending) error {
	subject, text, html := mail.TransferOTPEmail("kalke", code, mail.TransferDetails{
		Amount:      pending.Amount,
		Destination: pending.DestinationDisplay,
		Holder:      pending.DestinationHolder,
		Currency:    "USD",
	})
	return s.mailer.Send(ctx, mail.Message{
		To:      email,
		Subject: subject,
		Text:    text,
		HTML:    html,
	})
}

var errBankUnavailable = errors.New("bank proxy not configured")
