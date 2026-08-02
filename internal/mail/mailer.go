package mail

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

type LogMailer struct {
	Log *slog.Logger
}

func (m LogMailer) Send(_ context.Context, msg Message) error {
	log := m.Log
	if log == nil {
		log = slog.Default()
	}
	log.Info("dev mail", "to", msg.To, "subject", msg.Subject, "text", msg.Text)
	return nil
}

func SignupOTPEmail(fromName, code string) (subject, text, html string) {
	subject = "Seu código kalke"
	text = fmt.Sprintf(
		"Olá,\n\nSeu código de verificação kalke é: %s\n\nEle expira em 15 minutos.\nSe você não pediu isso, ignore este email.\n\n— kalke\n",
		code,
	)
	html = fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;padding:0;background:#0f1410;color:#e8efe6;font-family:Georgia,'Times New Roman',serif;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#0f1410;padding:32px 16px;">
    <tr><td align="center">
      <table role="presentation" width="100%%" style="max-width:480px;background:#182019;border:1px solid #2c3a2e;border-radius:12px;padding:28px;">
        <tr><td style="font-size:28px;letter-spacing:0.04em;color:#c8e6c0;">kalke</td></tr>
        <tr><td style="padding-top:18px;font-size:16px;line-height:1.5;color:#d7e4d3;">
          Seu código de verificação:
        </td></tr>
        <tr><td align="center" style="padding:22px 0;">
          <div style="display:inline-block;font-size:32px;letter-spacing:0.35em;font-weight:700;color:#f4fff0;background:#101610;border:1px solid #3a4d3d;border-radius:10px;padding:14px 22px;">%s</div>
        </td></tr>
        <tr><td style="font-size:13px;line-height:1.5;color:#9bb09a;">
          Expira em 15 minutos. Se você não pediu isso, pode ignorar este email.
        </td></tr>
        <tr><td style="padding-top:24px;font-size:12px;color:#6f8570;">— %s</td></tr>
      </table>
    </td></tr>
  </table>
</body></html>`, code, fromName)
	return subject, text, html
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
