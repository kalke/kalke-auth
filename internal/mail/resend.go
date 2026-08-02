package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Resend is a free-tier friendly alternative when Mailgun is unavailable.
type Resend struct {
	APIKey string
	From   string
	HTTP   *http.Client
}

func NewResend(apiKey, from string) *Resend {
	return &Resend{
		APIKey: strings.TrimSpace(apiKey),
		From:   strings.TrimSpace(from),
		HTTP:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *Resend) Send(ctx context.Context, msg Message) error {
	if r.APIKey == "" || r.From == "" {
		return fmt.Errorf("resend not configured")
	}
	payload := map[string]any{
		"from":    r.From,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"text":    msg.Text,
	}
	if msg.HTML != "" {
		payload["html"] = msg.HTML
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("resend: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
