package httpapi

import "strings"

// privilegedPermissions are never granted to site-signup / non-allowlisted users.
var privilegedPermissions = map[string]struct{}{
	"admin":      {},
	"bank:write": {},
}

// keycloakNoisePermissions appear in JWT realm-role claims but are not app permissions.
var keycloakNoisePermissions = map[string]struct{}{
	"offline_access":       {},
	"uma_authorization":    {},
	"manage-account":       {},
	"manage-account-links": {},
	"view-profile":         {},
}

func (s *Server) isAdminEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, allowed := range s.cfg.AdminEmails {
		if email == strings.ToLower(strings.TrimSpace(allowed)) {
			return true
		}
	}
	return false
}

// effectivePermissions drops privileged roles unless the email is allowlisted,
// and strips Keycloak built-in roles from the permissions claim.
func (s *Server) effectivePermissions(email string, perms []string) []string {
	if len(perms) == 0 {
		return nil
	}
	allowPriv := s.isAdminEmail(email)
	out := make([]string, 0, len(perms))
	seen := map[string]struct{}{}
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "default-roles-") {
			continue
		}
		if _, noise := keycloakNoisePermissions[p]; noise {
			continue
		}
		if _, priv := privilegedPermissions[p]; priv && !allowPriv {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
