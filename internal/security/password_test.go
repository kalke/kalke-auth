package security

import "testing"

func TestPasswordStrengthError(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want string
	}{
		{"empty", "", "password too short"},
		{"too_short_ascii", "short", "password too short"},
		{"nine_runes", "abcdefghi", "password too short"},
		{"ten_digits", "1234567890", "password needs a letter"},
		{"ten_letters", "abcdefghij", "password needs a number"},
		{"letters_and_symbol", "abcdefghij!", "password needs a number"},
		{"mixed_ok", "abcdefghi1", ""},
		{"mixed_caps", "SenhaForte1", ""},
		{"accents_plus_digit", "áéíóúñçãõ1", ""},
		{"eleven_ok", "abcdefghij1", ""},
		{"space_and_digit", "abcd efgh1", ""},
		{"only_symbols", "!@#$%^&*()", "password needs a letter"},
		{"nine_symbols", "!@#$%^&*(", "password too short"},
		{"unicode_digit_nd", "abcdefghij٢", "password needs a number"}, // Arabic-Indic 2 is not 0-9
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PasswordStrengthError(tc.pw)
			if got != tc.want {
				t.Fatalf("pw=%q: got %q want %q", tc.pw, got, tc.want)
			}
		})
	}
}

func TestMinPasswordRunes(t *testing.T) {
	if MinPasswordRunes != 10 {
		t.Fatalf("MinPasswordRunes=%d want 10", MinPasswordRunes)
	}
}
