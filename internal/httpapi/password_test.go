package httpapi

import "testing"

func TestPasswordChangeValidation(t *testing.T) {
	cases := []struct {
		current, next string
		want          string
	}{
		{"", "abcdefghij", "current and new password required"},
		{"oldpass1234", "", "current and new password required"},
		{"oldpass1234", "short", "password too short"},
		{"samepass12", "samepass12", "new password must differ"},
		{"oldpass1234", "newpass1234", ""},
	}
	for _, tc := range cases {
		got := passwordChangeValidationError(tc.current, tc.next)
		if got != tc.want {
			t.Fatalf("current=%q new=%q: got %q want %q", tc.current, tc.next, got, tc.want)
		}
	}
}
