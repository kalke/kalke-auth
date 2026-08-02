package httpapi

import "testing"

func TestIsPublicOIDCPath(t *testing.T) {
	allow := []string{
		"/realms/kalke/.well-known/openid-configuration",
		"/realms/kalke/protocol/openid-connect/certs",
		"/realms/kalke/protocol/openid-connect/certs/extra",
	}
	deny := []string{
		"/resources/abc",
		"/realms/kalke/protocol/openid-connect/token",
		"/realms/kalke/protocol/openid-connect/auth",
		"/admin",
		"/realms/master/.well-known/openid-configuration",
		"/v1/auth/login",
	}
	for _, p := range allow {
		if !isPublicOIDCPath(p) {
			t.Fatalf("expected allow %s", p)
		}
	}
	for _, p := range deny {
		if isPublicOIDCPath(p) {
			t.Fatalf("expected deny %s", p)
		}
	}
}
