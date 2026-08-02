package httpapi

import "strings"

// privilegedPermissions are never granted to site-signup / non-allowlisted users.
var privilegedPermissions = map[string]struct{}{
	"admin":      {},
	"bank:write": {},
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

// effectivePermissions drops privileged roles unless the email is allowlisted.
func (s *Server) effectivePermissions(email string, perms []string) []string {
	if len(perms) == 0 {
		return nil
	}
	allowPriv := s.isAdminEmail(email)
	out := make([]string, 0, len(perms))
	seen := map[string]struct{}{}
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" {
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
