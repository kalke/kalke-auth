package keycloak

import "testing"

func TestSplitDisplayName(t *testing.T) {
	first, last := SplitDisplayName("  Henrique Kalke  ")
	if first != "Henrique" || last != "Kalke" {
		t.Fatalf("got %q %q", first, last)
	}
	first, last = SplitDisplayName("Ada")
	if first != "Ada" || last != "" {
		t.Fatalf("got %q %q", first, last)
	}
	first, last = SplitDisplayName("Ana Maria Silva")
	if first != "Ana" || last != "Maria Silva" {
		t.Fatalf("got %q %q", first, last)
	}
}

func TestRealmUserDisplayName(t *testing.T) {
	u := RealmUser{FirstName: "Henrique", LastName: "Kalke"}
	if u.DisplayName() != "Henrique Kalke" {
		t.Fatalf("got %q", u.DisplayName())
	}
}
