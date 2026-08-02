package security

import (
	"unicode"
	"unicode/utf8"
)

const MinPasswordRunes = 10

// PasswordStrengthError returns a stable API error string when password is weak.
// Rules match the kalke playground: ≥10 runes, at least one letter, at least one digit (0-9).
func PasswordStrengthError(password string) string {
	if utf8.RuneCountInString(password) < MinPasswordRunes {
		return "password too short"
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	if !hasLetter {
		return "password needs a letter"
	}
	if !hasDigit {
		return "password needs a number"
	}
	return ""
}
