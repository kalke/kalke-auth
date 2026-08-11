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
