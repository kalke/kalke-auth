package security

import "testing"

func TestSealOpen(t *testing.T) {
	pepper := []byte("test-pepper")
	sealed, err := Seal(pepper, "SuperSecret1!")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(pepper, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got != "SuperSecret1!" {
		t.Fatalf("got %q", got)
	}
}
