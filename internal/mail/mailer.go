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
	OTPSignup         OTPKind = "signup"
	OTPPasswordless   OTPKind = "passwordless"
	OTPReset          OTPKind = "reset"
	OTPEmailChange    OTPKind = "email_change"
	OTPTransfer       OTPKind = "transfer"
	OTPPasswordChange OTPKind = "password_change"
)

type TransferDetails struct {
	Amount      string
	Destination string
	Holder      string
	Currency    string
}

// TransferReceiptDetails is used for post-transfer sent/received notices.
type TransferReceiptDetails struct {
	Amount      string
	Currency    string
	Counterparty string // destination (sent) or origin (received) display line
	Holder      string
	Memo        string
	Balance     string
	When        string
}

func SignupOTPEmail(fromName, code string) (subject, text, html string) {
	return OTPEmail(OTPSignup, fromName, code)
}

func PasswordlessOTPEmail(fromName, code string) (subject, text, html string) {
	return OTPEmail(OTPPasswordless, fromName, code)
}

func ResetOTPEmail(fromName, code string) (subject, text, html string) {
	return OTPEmail(OTPReset, fromName, code)
}

// TransferOTPEmail builds a confirmation mail for a pending demo bank transfer.
func TransferOTPEmail(fromName, code string, details TransferDetails) (subject, text, html string) {
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		fromName = "kalke"
	}
	amount := strings.TrimSpace(details.Amount)
	if amount == "" {
		amount = "—"
	}
	currency := strings.TrimSpace(details.Currency)
	if currency == "" {
		currency = "USD"
	}
	destination := strings.TrimSpace(details.Destination)
	if destination == "" {
		destination = "—"
	}
	holder := strings.TrimSpace(details.Holder)
	destLine := destination
	if holder != "" {
		destLine = fmt.Sprintf("%s · %s", destination, holder)
	}
	amountLine := fmt.Sprintf("%s %s", currency, amount)

	subject = "Confirmar transferência · kalke"
	text = fmt.Sprintf(
		"Olá,\n\nVocê pediu uma transferência na demo bancária kalke.\n\nValor: %s\nDestino: %s\n\nUse este código para confirmar:\n%s\n\nO código expira em 15 minutos. Se você não pediu essa transferência, ignore este email — nada será enviado.\n\n— %s\n",
		amountLine, destLine, code, fromName,
	)
	html = fmt.Sprintf(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#100e0c;color:#f2ece4;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#100e0c;padding:36px 16px;">
    <tr><td align="center">
      <table role="presentation" width="100%%" style="max-width:480px;background:#211c18;border:1px solid rgba(242,236,228,0.12);border-radius:4px;">
        <tr><td style="padding:28px 26px 8px;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:22px;letter-spacing:0.06em;color:#e7a339;">kalke</td></tr>
        <tr><td style="padding:0 26px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.08em;text-transform:uppercase;color:#4fb6b0;">Demo bancária</td></tr>
        <tr><td style="padding:18px 26px 0;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:20px;font-weight:600;line-height:1.3;color:#f2ece4;">Confirmar transferência</td></tr>
        <tr><td style="padding:12px 26px 0;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:15px;line-height:1.55;color:#f2ece4;">
          Você pediu uma transferência na demo. Para concluir, digite o código abaixo no site.
        </td></tr>
        <tr><td style="padding:20px 26px 0;">
          <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#15120f;border:1px solid rgba(242,236,228,0.12);border-radius:4px;">
            <tr>
              <td style="padding:14px 16px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;">Valor</td>
              <td align="right" style="padding:14px 16px;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:15px;font-weight:600;color:#f2ece4;">%s</td>
            </tr>
            <tr>
              <td style="padding:0 16px 14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;border-top:1px solid rgba(242,236,228,0.08);padding-top:14px;">Destino</td>
              <td align="right" style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:14px;line-height:1.4;color:#f2ece4;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
            </tr>
          </table>
        </td></tr>
        <tr><td align="center" style="padding:26px 26px 10px;">
          <div style="display:inline-block;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:30px;letter-spacing:0.4em;font-weight:600;color:#f2ece4;background:#15120f;border:1px solid rgba(231,163,57,0.45);border-radius:4px;padding:14px 20px 14px 28px;">%s</div>
        </td></tr>
        <tr><td style="padding:8px 26px 0;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:13px;line-height:1.5;color:#9c948a;">
          Expira em 15 minutos. Se você não pediu essa transferência, ignore este email — nenhum valor será movido.
        </td></tr>
        <tr><td style="padding:28px 26px;border-top:1px solid rgba(242,236,228,0.12);margin-top:20px;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:12px;color:#9c948a;">— %s</td></tr>
      </table>
    </td></tr>
  </table>
</body></html>`, amountLine, destLine, code, fromName)
	return subject, text, html
}

func formatAmountLine(currency, amount string) string {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		amount = "—"
	}
	currency = strings.TrimSpace(currency)
	if currency == "" {
		currency = "USD"
	}
	return currency + " " + amount
}

func formatPartyLine(display, holder string) string {
	display = strings.TrimSpace(display)
	if display == "" {
		display = "—"
	}
	holder = strings.TrimSpace(holder)
	if holder == "" {
		return display
	}
	return display + " · " + holder
}

func transferReceiptShell(headline, lead, amountLine, partyLabel, partyLine, memo, balanceLabel, balance, when, fromName string) (text, html string) {
	memoBlockText := ""
	memoRowHTML := ""
	if strings.TrimSpace(memo) != "" {
		memoBlockText = fmt.Sprintf("\nMemo: %s", strings.TrimSpace(memo))
		memoRowHTML = fmt.Sprintf(`
            <tr>
              <td style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;border-top:1px solid rgba(242,236,228,0.08);">Memo</td>
              <td align="right" style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:14px;line-height:1.4;color:#f2ece4;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
            </tr>`, strings.TrimSpace(memo))
	}
	balance = strings.TrimSpace(balance)
	when = strings.TrimSpace(when)
	if when == "" {
		when = "—"
	}
	balanceText := ""
	balanceRowHTML := ""
	if balance != "" {
		balanceText = fmt.Sprintf("\n%s: %s", balanceLabel, balance)
		balanceRowHTML = fmt.Sprintf(`
            <tr>
              <td style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
              <td align="right" style="padding:0 16px 14px;padding-top:14px;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:14px;font-weight:600;color:#f2ece4;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
            </tr>`, balanceLabel, balance)
	}
	text = fmt.Sprintf(
		"Olá,\n\n%s\n\nValor: %s\n%s: %s%s%s\nQuando: %s\n\nDemo bancária kalke — fundos virtuais, nada é liquidado de verdade.\n\n— %s\n",
		lead, amountLine, partyLabel, partyLine, memoBlockText, balanceText, when, fromName,
	)
	html = fmt.Sprintf(`<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#100e0c;color:#f2ece4;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#100e0c;padding:36px 16px;">
    <tr><td align="center">
      <table role="presentation" width="100%%" style="max-width:480px;background:#211c18;border:1px solid rgba(242,236,228,0.12);border-radius:4px;">
        <tr><td style="padding:28px 26px 8px;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:22px;letter-spacing:0.06em;color:#e7a339;">kalke</td></tr>
        <tr><td style="padding:0 26px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.08em;text-transform:uppercase;color:#4fb6b0;">Demo bancária</td></tr>
        <tr><td style="padding:18px 26px 0;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:20px;font-weight:600;line-height:1.3;color:#f2ece4;">%s</td></tr>
        <tr><td style="padding:12px 26px 0;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:15px;line-height:1.55;color:#f2ece4;">%s</td></tr>
        <tr><td style="padding:20px 26px 0;">
          <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#15120f;border:1px solid rgba(242,236,228,0.12);border-radius:4px;">
            <tr>
              <td style="padding:14px 16px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;">Valor</td>
              <td align="right" style="padding:14px 16px;font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:15px;font-weight:600;color:#f2ece4;">%s</td>
            </tr>
            <tr>
              <td style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
              <td align="right" style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:14px;line-height:1.4;color:#f2ece4;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
            </tr>%s%s
            <tr>
              <td style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:12px;letter-spacing:0.06em;text-transform:uppercase;color:#9c948a;border-top:1px solid rgba(242,236,228,0.08);">Quando</td>
              <td align="right" style="padding:0 16px 14px;padding-top:14px;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:13px;line-height:1.4;color:#f2ece4;border-top:1px solid rgba(242,236,228,0.08);">%s</td>
            </tr>
          </table>
        </td></tr>
        <tr><td style="padding:18px 26px 0;font-family:'IBM Plex Sans','Helvetica Neue',Arial,sans-serif;font-size:13px;line-height:1.5;color:#9c948a;">
          Fundos virtuais para demonstração — nada é liquidado de verdade.
        </td></tr>
        <tr><td style="padding:28px 26px;border-top:1px solid rgba(242,236,228,0.12);font-family:'JetBrains Mono','IBM Plex Mono',ui-monospace,Consolas,monospace;font-size:12px;color:#9c948a;">— %s</td></tr>
      </table>
    </td></tr>
  </table>
</body></html>`, headline, lead, amountLine, partyLabel, partyLine, memoRowHTML, balanceRowHTML, when, fromName)
	return text, html
}

// TransferSentEmail notifies the sender that a demo transfer left their account.
func TransferSentEmail(fromName string, details TransferReceiptDetails) (subject, text, html string) {
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		fromName = "kalke"
	}
	amountLine := formatAmountLine(details.Currency, details.Amount)
	partyLine := formatPartyLine(details.Counterparty, details.Holder)
	balanceFmt := ""
	if strings.TrimSpace(details.Balance) != "" {
		balanceFmt = formatAmountLine(details.Currency, details.Balance)
	}
	subject = "Transferência enviada · kalke"
	text, html = transferReceiptShell(
		"Transferência enviada",
		"Sua transferência na demo bancária kalke foi concluída.",
		amountLine,
		"Destino",
		partyLine,
		details.Memo,
		"Saldo origem",
		balanceFmt,
		details.When,
		fromName,
	)
	return subject, text, html
}

// TransferReceivedEmail notifies the recipient that a demo transfer arrived.
func TransferReceivedEmail(fromName string, details TransferReceiptDetails) (subject, text, html string) {
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		fromName = "kalke"
	}
	amountLine := formatAmountLine(details.Currency, details.Amount)
	partyLine := formatPartyLine(details.Counterparty, details.Holder)
	balanceFmt := ""
	if strings.TrimSpace(details.Balance) != "" {
		balanceFmt = formatAmountLine(details.Currency, details.Balance)
	}
	subject = "Transferência recebida · kalke"
	text, html = transferReceiptShell(
		"Transferência recebida",
		"Você recebeu uma transferência na demo bancária kalke.",
		amountLine,
		"Origem",
		partyLine,
		details.Memo,
		"Saldo atual",
		balanceFmt,
		details.When,
		fromName,
	)
	return subject, text, html
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
	case OTPPasswordChange:
		subjectLine = "Confirmar nova senha · kalke"
		headline = "Confirmar nova senha"
		lead = "Use este código para confirmar a troca de senha da sua conta kalke:"
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

// MaskEmail returns a privacy-preserving display form like a***@domain.com.
func MaskEmail(email string) string {
	email = NormalizeEmail(email)
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return "***"
	}
	local, domain := email[:at], email[at+1:]
	if len(local) == 1 {
		return local + "***@" + domain
	}
	return string(local[0]) + "***@" + domain
}
