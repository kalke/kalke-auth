package signup

import (
	"testing"
	"time"
)

func TestOTPRoundTrip(t *testing.T) {
	s := &Store{pepper: []byte("pepper")}
	p, otp, err := s.NewPending("Henrique", "user@example.com", "password1234")
	if err != nil {
		t.Fatal(err)
	}
	if len(otp) != 6 {
		t.Fatalf("otp len %d", len(otp))
	}
	if err := s.VerifyOTP(&p, otp); err != nil {
		t.Fatal(err)
	}
	plain, err := s.Password(p)
	if err != nil || plain != "password1234" {
		t.Fatalf("password %q err=%v", plain, err)
	}
}

func TestResendCooldown(t *testing.T) {
	s := &Store{pepper: []byte("pepper")}
	p := Pending{LastSentAt: time.Now().UTC()}
	if wait, ok := s.ResendAllowed(p); ok || wait <= 0 {
		t.Fatalf("expected cooldown, wait=%v ok=%v", wait, ok)
	}
	p.LastSentAt = time.Now().UTC().Add(-3 * time.Minute)
	if _, ok := s.ResendAllowed(p); !ok {
		t.Fatal("expected resend allowed")
	}
}

func TestInvalidOTP(t *testing.T) {
	s := &Store{pepper: []byte("pepper")}
	p, _, err := s.NewPending("A", "a@b.com", "password1234")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyOTP(&p, "000000"); err != ErrInvalidOTP {
		t.Fatalf("got %v", err)
	}
}
