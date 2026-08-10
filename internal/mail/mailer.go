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

type OTPKind string

const (
	OTPSignup       OTPKind = "signup"
	OTPPasswordless OTPKind = "passwordless"
	OTPReset        OTPKind = "reset"
	OTPEmailChange  OTPKind = "email_change"
)

func SignupOTPEmail(fromName, code string) (subject, text, html string) {
	return OTPEmail(OTPSignup, fromName, code)
}

func PasswordlessOTPEmail(fromName, code string) (subject, text, html string) {
	return OTPEmail(OTPPasswordless, fromName, code)
}

func ResetOTPEmail(fromName, code string) (subject, text, html string) {
	return OTPEmail(OTPReset, fromName, code)
}

// OTPEmail builds branded kalke OTP mail (warm dark / terminal identity).
func OTPEmail(kind OTPKind, fromName, code string) (subject, text, html string) {
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		fromName = "kalke"
	}
	var headline, lead, subjectLine string
	switch kind {
	case OTPPasswordless:
		subjectLine = "Seu código de acesso kalke"
		headline = "Entrar sem senha"
		lead = "Use este código para entrar na sua conta kalke:"
	case OTPReset:
		subjectLine = "Redefinir senha kalke"
		headline = "Redefinir senha"
		lead = "Use este código para escolher uma nova senha:"
	case OTPEmailChange:
		subjectLine = "Confirmar novo email kalke"
		headline = "Confirmar novo email"
		lead = "Use este código para confirmar o novo email da sua conta kalke:"
	default:
		subjectLine = "Seu código kalke"
		headline = "Verificação de email"
		lead = "Seu código de verificação:"
	}
	subject = subjectLine
	text = fmt.Sprintf(
		"Olá,\n\n%s %s\n\nEle expira em 15 minutos.\nSe você não pediu isso, ignore este email.\n\n— %s\n",
		lead, code, fromName,
	)
	html = fmt.Sprintf(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"></head>
<body style="margin:0;padding:0;background:#100e0c;color:#f2ece4;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#100e0c;padding:36px 16px;">
    <tr><td align="center">
      <table role="presentation" width="100%%" style="max-width:480px;background:#211c18;border:1px solid rgba(242,236,228,0.12);border-radius:4px;padding:28px 26px;">
        <tr><td style="font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:22px;letter-spacing:0.06em;color:#e7a339;">kalke</td></tr>
        <tr><td style="padding-top:6px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.08em;text-transform:uppercase;color:#4fb6b0;">%s</td></tr>
        <tr><td style="padding-top:20px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:15px;line-height:1.55;color:#f2ece4;">%s</td></tr>
        <tr><td align="center" style="padding:26px 0 10px;">
          <div style="display:inline-block;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:30px;letter-spacing:0.4em;font-weight:600;color:#f2ece4;background:#15120f;border:1px solid rgba(231,163,57,0.45);border-radius:4px;padding:14px 20px 14px 28px;">%s</div>
        </td></tr>
        <tr><td style="padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:13px;line-height:1.5;color:#9c948a;">
          Expira em 15 minutos. Se você não pediu isso, ignore este email.
        </td></tr>
        <tr><td style="padding-top:28px;border-top:1px solid rgba(242,236,228,0.12);font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:12px;color:#9c948a;">— %s</td></tr>
      </table>
    </td></tr>
  </table>
</body></html>`, headline, lead, code, fromName)
	return subject, text, html
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
