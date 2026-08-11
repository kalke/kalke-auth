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

func TestJoinBankProxyPath(t *testing.T) {
	cases := []struct {
		prefix, display, cep, id, want string
	}{
		{"/v1/me/accounts/", "100123-4", "", "", "/v1/me/accounts/100123-4"},
		{"/v1/me/accounts", "100123-4", "", "", "/v1/me/accounts/100123-4"},
		{"/v1/cep/", "", "01310100", "", "/v1/cep/01310100"},
		{"/v1/cep", "", "01310-100", "", "/v1/cep/01310-100"},
		{"/v1/me/transactions/", "", "", "tx-1", "/v1/me/transactions/tx-1"},
	}
	for _, tc := range cases {
		got := joinBankProxyPath(tc.prefix, tc.display, tc.cep, tc.id)
		if got != tc.want {
			t.Fatalf("joinBankProxyPath(%q,%q,%q,%q)=%q want %q",
				tc.prefix, tc.display, tc.cep, tc.id, got, tc.want)
		}
	}
}
