package mail

import "testing"

func TestMaskEmail(t *testing.T) {
	if got := MaskEmail("henrique@icloud.com"); got != "h***@icloud.com" {
		t.Fatalf("got %q", got)
	}
	if got := MaskEmail("a@b.co"); got != "a***@b.co" {
		t.Fatalf("got %q", got)
	}
}

func TestTransferOTPEmail(t *testing.T) {
	subject, text, html := TransferOTPEmail("kalke", "123456", TransferDetails{
		Amount:      "4000.00",
		Destination: "2-9",
		Holder:      "Maria",
		Currency:    "USD",
	})
	if subject == "" || !containsAll(text, "123456", "4000.00", "2-9") {
		t.Fatalf("text incomplete: %s", text)
	}
	if !containsAll(html, "123456", "USD 4000.00", "Confirmar transferência") {
		t.Fatalf("html incomplete")
	}
}

func TestTransferSentEmail(t *testing.T) {
	subject, text, html := TransferSentEmail("kalke", TransferReceiptDetails{
		Amount:       "25.50",
		Currency:     "USD",
		Counterparty: "2-9",
		Holder:       "Maria",
		Memo:         "demo payment",
		Balance:      "9974.50",
		When:         "2026-08-11 13:00 UTC",
	})
	if subject != "Transferência enviada · kalke" {
		t.Fatalf("subject %q", subject)
	}
	if !containsAll(text, "25.50", "2-9", "demo payment", "9974.50") {
		t.Fatalf("text incomplete: %s", text)
	}
	if !containsAll(html, "Transferência enviada", "Destino", "USD 25.50") {
		t.Fatalf("html incomplete")
	}
	_, textNoMemo, htmlNoMemo := TransferSentEmail("kalke", TransferReceiptDetails{
		Amount:       "1.00",
		Currency:     "USD",
		Counterparty: "3-1",
		When:         "now",
	})
	if contains(textNoMemo, "Memo:") || contains(htmlNoMemo, ">Memo<") {
		t.Fatal("empty memo should be omitted")
	}
}

func TestTransferReceivedEmail(t *testing.T) {
	subject, text, html := TransferReceivedEmail("kalke", TransferReceiptDetails{
		Amount:       "25.50",
		Currency:     "USD",
		Counterparty: "1-7",
		Holder:       "Henrique",
		Balance:      "10025.50",
		When:         "2026-08-11 13:00 UTC",
	})
	if subject != "Transferência recebida · kalke" {
		t.Fatalf("subject %q", subject)
	}
	if !containsAll(text, "25.50", "1-7", "Origem") {
		t.Fatalf("text incomplete: %s", text)
	}
	if !containsAll(html, "Transferência recebida", "Saldo atual", "USD 10025.50") {
		t.Fatalf("html incomplete")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
