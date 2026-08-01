package security

import "testing"

func TestPATRoundTrip(t *testing.T) {
	pepper := []byte("test-pepper")
	plain, prefix, hash, err := NewPAT(pepper)
	if err != nil {
		t.Fatal(err)
	}
	if !IsPAT(plain) {
		t.Fatalf("expected pat prefix: %s", plain)
	}
	got, err := PATPrefixFromToken(plain)
	if err != nil || got != prefix {
		t.Fatalf("prefix mismatch: %v %s", err, got)
	}
	if !CheckSecret(pepper, plain, hash) {
		t.Fatal("hash check failed")
	}
	if CheckSecret(pepper, plain+"x", hash) {
		t.Fatal("expected mismatch")
	}
}
