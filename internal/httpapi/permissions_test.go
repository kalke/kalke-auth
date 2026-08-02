package httpapi

import (
	"testing"

	"github.com/kalke/kalke-auth/internal/config"
)

func TestEffectivePermissions(t *testing.T) {
	s := &Server{cfg: config.Config{AdminEmails: []string{"henriquekalke@icloud.com"}}}

	got := s.effectivePermissions("henriquekalke@icloud.com", []string{"admin", "extract:write", "bank:write"})
	if len(got) != 3 {
		t.Fatalf("admin email should keep privileged roles: %v", got)
	}

	got = s.effectivePermissions("other@example.com", []string{"admin", "extract:write", "bank:write"})
	if len(got) != 1 || got[0] != "extract:write" {
		t.Fatalf("non-admin must lose privileged roles: %v", got)
	}

	if s.isAdminEmail("") || s.isAdminEmail("other@example.com") {
		t.Fatal("unexpected admin email match")
	}
	if !s.isAdminEmail("HenriqueKalke@icloud.com") {
		t.Fatal("admin email should be case-insensitive")
	}
}
