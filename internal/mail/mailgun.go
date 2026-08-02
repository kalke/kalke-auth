package mail

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Mailgun struct {
	APIKey  string
	Domain  string
	From    string
	BaseURL string
	HTTP    *http.Client
}

func NewMailgun(apiKey, domain, from string) *Mailgun {
	return &Mailgun{
		APIKey:  strings.TrimSpace(apiKey),
		Domain:  strings.TrimSpace(domain),
		From:    strings.TrimSpace(from),
		BaseURL: "https://api.mailgun.net",
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (m *Mailgun) Send(ctx context.Context, msg Message) error {
	if m.APIKey == "" || m.Domain == "" || m.From == "" {
		return fmt.Errorf("mailgun not configured")
	}
	form := url.Values{}
	form.Set("from", m.From)
	form.Set("to", msg.To)
	form.Set("subject", msg.Subject)
	form.Set("text", msg.Text)
	if msg.HTML != "" {
		form.Set("html", msg.HTML)
	}
	endpoint := strings.TrimRight(m.BaseURL, "/") + "/v3/" + m.Domain + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth("api", m.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mailgun: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
