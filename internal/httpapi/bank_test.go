package httpapi

import "testing"

func TestHasBankPermission(t *testing.T) {
	if !hasBankPermission([]string{"bank:demo"}) {
		t.Fatal("bank:demo should allow")
	}
	if !hasBankPermission([]string{"bank:write"}) {
		t.Fatal("bank:write should allow")
	}
	if !hasBankPermission([]string{"admin"}) {
		t.Fatal("admin should allow")
	}
	if hasBankPermission([]string{"extract:write"}) {
		t.Fatal("extract:write alone should deny")
	}
}
