package httpapi

import "testing"

func TestIsPublicOIDCPath(t *testing.T) {
	allow := []string{
		"/realms/kalke/.well-known/openid-configuration",
		"/realms/kalke/protocol/openid-connect/certs",
		"/realms/kalke/protocol/openid-connect/certs/extra",
		"/realms/kalke/protocol/openid-connect/auth",
		"/realms/kalke/broker/google/endpoint",
		"/realms/kalke/login-actions/authenticate",
		"/resources/abc/login.css",
	}
	deny := []string{
		"/realms/kalke/protocol/openid-connect/token",
		"/realms/kalke/protocol/openid-connect/logout",
		"/admin",
		"/realms/master/.well-known/openid-configuration",
		"/v1/auth/login",
		"/realms/kalke",
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
