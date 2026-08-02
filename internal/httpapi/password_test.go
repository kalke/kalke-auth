package httpapi

import (
	"testing"

	"github.com/kalke/kalke-auth/internal/security"
)

func TestPasswordChangeValidation(t *testing.T) {
	cases := []struct {
		name          string
		current, next string
		want          string
	}{
		{"missing_current", "", "abcdefghij1", "current and new password required"},
		{"missing_next", "oldpass1234", "", "current and new password required"},
		{"both_empty", "", "", "current and new password required"},
		{"too_short", "oldpass1234", "short", "password too short"},
		{"needs_number", "oldpass1234", "abcdefghij", "password needs a number"},
		{"needs_letter", "oldpass1234", "1234567890", "password needs a letter"},
		{"same_as_current", "samepass12", "samepass12", "new password must differ"},
		{"ok", "oldpass1234", "newpass1234", ""},
		{"ok_unicode", "oldpass1234", "áéíóúñçãõ1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := passwordChangeValidationError(tc.current, tc.next)
			if got != tc.want {
				t.Fatalf("current=%q new=%q: got %q want %q", tc.current, tc.next, got, tc.want)
			}
		})
	}
}

func TestSignupAndChangeSharePasswordStrength(t *testing.T) {
	// signupStart and changePassword both call security.PasswordStrengthError.
	weak := []string{"short", "abcdefghij", "1234567890"}
	for _, pw := range weak {
		if security.PasswordStrengthError(pw) == "" {
			t.Fatalf("expected strength error for %q", pw)
		}
		if msg := passwordChangeValidationError("other-pass12", pw); msg == "" {
			t.Fatalf("change-password should reject %q", pw)
		}
	}
	if security.PasswordStrengthError("goodpass12") != "" {
		t.Fatal("expected strong password to pass")
	}
}
