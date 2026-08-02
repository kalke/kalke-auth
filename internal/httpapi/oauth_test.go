package httpapi

import (
	"testing"

	"github.com/kalke/kalke-auth/internal/config"
)

func TestSafeReturnURL(t *testing.T) {
	s := &Server{cfg: config.Config{
		CORSOrigins:     []string{"https://kalke.dev", "https://www.kalke.dev"},
		OAuthSuccessURL: "https://kalke.dev/playground",
	}}
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://kalke.dev/playground"},
		{"https://kalke.dev/playground", "https://kalke.dev/playground"},
		{"https://kalke.dev/playground?x=1", "https://kalke.dev/playground?x=1"},
		{"https://evil.example/phish", "https://kalke.dev/playground"},
		{"http://kalke.dev/playground", "https://kalke.dev/playground"},
		{"javascript:alert(1)", "https://kalke.dev/playground"},
	}
	for _, tc := range cases {
		if got := s.safeReturnURL(tc.in); got != tc.want {
			t.Fatalf("safeReturnURL(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestOAuthProviderAllowlist(t *testing.T) {
	if _, ok := oauthProviders["google"]; !ok {
		t.Fatal("google missing")
	}
	if _, ok := oauthProviders["facebook"]; ok {
		t.Fatal("facebook should not be allowed")
	}
}
