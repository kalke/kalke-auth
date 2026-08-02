package otp

import (
	"testing"
	"time"
)

func TestOTPRoundTrip(t *testing.T) {
	s := &Store{pepper: []byte("pepper"), prefix: "test:"}
	c, otp, err := s.New("user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(otp) != 6 {
		t.Fatalf("otp len %d", len(otp))
	}
	if err := s.Verify(&c, otp); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidOTP(t *testing.T) {
	s := &Store{pepper: []byte("pepper"), prefix: "test:"}
	c, _, err := s.New("a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(&c, "000000"); err != ErrInvalidOTP {
		t.Fatalf("got %v", err)
	}
}

func TestResendCooldown(t *testing.T) {
	s := &Store{pepper: []byte("pepper"), prefix: "test:"}
	c := Challenge{LastSentAt: time.Now().UTC()}
	if wait, ok := s.ResendAllowed(c); ok || wait <= 0 {
		t.Fatalf("expected cooldown, wait=%v ok=%v", wait, ok)
	}
	c.LastSentAt = time.Now().UTC().Add(-3 * time.Minute)
	if _, ok := s.ResendAllowed(c); !ok {
		t.Fatal("expected resend allowed")
	}
}
