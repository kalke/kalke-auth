package httpapi

import (
	"testing"

	"github.com/kalke/kalke-auth/internal/config"
)

func contains(perms []string, want string) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

func TestEffectivePermissions(t *testing.T) {
	s := &Server{cfg: config.Config{AdminEmails: []string{"henriquekalke@icloud.com"}}}

	got := s.effectivePermissions("henriquekalke@icloud.com", []string{"admin", "extract:write", "bank:write"})
	if !contains(got, "admin") || !contains(got, "extract:write") || !contains(got, "bank:write") || !contains(got, "bank:demo") {
		t.Fatalf("admin email should keep privileged roles + bank:demo: %v", got)
	}

	got = s.effectivePermissions("other@example.com", []string{"admin", "extract:write", "bank:write"})
	if contains(got, "admin") || contains(got, "bank:write") {
		t.Fatalf("non-admin must lose privileged roles: %v", got)
	}
	if !contains(got, "extract:write") || !contains(got, "bank:demo") {
		t.Fatalf("non-admin should keep extract + bank:demo: %v", got)
	}

	got = s.effectivePermissions("other@example.com", []string{
		"offline_access", "default-roles-kalke", "uma_authorization", "extract:write",
	})
	if !contains(got, "extract:write") || !contains(got, "bank:demo") || len(got) != 2 {
		t.Fatalf("keycloak noise roles must be stripped: %v", got)
	}

	got = s.effectivePermissions("other@example.com", nil)
	if len(got) != 1 || got[0] != "bank:demo" {
		t.Fatalf("empty perms should still grant bank:demo: %v", got)
	}

	if s.isAdminEmail("") || s.isAdminEmail("other@example.com") {
		t.Fatal("unexpected admin email match")
	}
	if !s.isAdminEmail("HenriqueKalke@icloud.com") {
		t.Fatal("admin email should be case-insensitive")
	}
}
