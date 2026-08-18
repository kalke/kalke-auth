package secrets

import (
	"os"
	"testing"
)

func TestWithoutExistingSkipsNonEmptyEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://compose/pinned")
	t.Setenv("KC_DB_URL", "")
	os.Unsetenv("KC_DB_PASSWORD")

	data := map[string]string{
		"DATABASE_URL":         "postgres://sm.example/db",
		"KC_DB_URL":            "jdbc:postgresql://sm.example/db",
		"KC_DB_PASSWORD":       "from-sm",
		"SESSION_SECRET":       "keep-me",
		"KALKE_SECRETS_LOADED": "from-sm",
	}
	got := WithoutExisting(data)
	if _, ok := got["DATABASE_URL"]; ok {
		t.Fatalf("DATABASE_URL should stay Compose-pinned, got %q", got["DATABASE_URL"])
	}
	if got["KC_DB_URL"] != "jdbc:postgresql://sm.example/db" {
		t.Fatalf("empty env should be filled from SM, got %q", got["KC_DB_URL"])
	}
	if got["KC_DB_PASSWORD"] != "from-sm" {
		t.Fatalf("unset env should be filled from SM, got %q", got["KC_DB_PASSWORD"])
	}
	if got["SESSION_SECRET"] != "keep-me" {
		t.Fatalf("missing key should pass through, got %q", got["SESSION_SECRET"])
	}
	if _, ok := got["KALKE_SECRETS_LOADED"]; ok {
		t.Fatal("sentinel must not be copied from the secret blob")
	}
}
