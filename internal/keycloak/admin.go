package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type AdminClient struct {
	internalBase string
	adminUser    string
	adminPass    string
	http         *http.Client
}

func NewAdmin(internalBase, adminUser, adminPass string) *AdminClient {
	return &AdminClient{
		internalBase: strings.TrimSuffix(internalBase, "/"),
		adminUser:    adminUser,
		adminPass:    adminPass,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *AdminClient) token(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", "admin-cli")
	form.Set("username", a.adminUser)
	form.Set("password", a.adminPass)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.internalBase+"/realms/master/protocol/openid-connect/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("admin token: %d", resp.StatusCode)
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return "", fmt.Errorf("admin token parse")
	}
	return tr.AccessToken, nil
}

type createUserBody struct {
	Username        string              `json:"username"`
	Email           string              `json:"email"`
	FirstName       string              `json:"firstName,omitempty"`
	Enabled         bool                `json:"enabled"`
	EmailVerified   bool                `json:"emailVerified"`
	RequiredActions []string            `json:"requiredActions"`
	Credentials     []map[string]any    `json:"credentials"`
	Attributes      map[string][]string `json:"attributes,omitempty"`
}

// CreateUser creates a realm user with password and grants extract:write.
// Used after email OTP verification — never grant admin via public signup.
func (a *AdminClient) CreateUser(ctx context.Context, name, email, password string) (userID string, err error) {
	tok, err := a.token(ctx)
	if err != nil {
		return "", err
	}
	payload := createUserBody{
		Username:        email,
		Email:           email,
		FirstName:       strings.TrimSpace(name),
		Enabled:         true,
		EmailVerified:   true,
		RequiredActions: []string{}, // OTP already verified; block VERIFY_PROFILE login traps
		Credentials: []map[string]any{
			{
				"type":      "password",
				"value":     password,
				"temporary": false,
			},
		},
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.internalBase+"/admin/realms/kalke/users", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("user exists")
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return "", fmt.Errorf("create user: %d %s", resp.StatusCode, string(b))
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("create user: missing location")
	}
	parts := strings.Split(strings.TrimRight(loc, "/"), "/")
	userID = parts[len(parts)-1]
	if err := a.clearRequiredActions(ctx, tok, userID); err != nil {
		return "", err
	}
	// Playground extract requires extract:write; assign explicitly so passwordless
	// sessions (which list direct roles) and JWT claims both see it.
	if err := a.assignRealmRole(ctx, tok, userID, "extract:write"); err != nil {
		return "", fmt.Errorf("assign extract:write: %w", err)
	}
	return userID, nil
}

func (a *AdminClient) clearRequiredActions(ctx context.Context, tok, userID string) error {
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.internalBase+"/admin/realms/kalke/users/"+url.PathEscape(userID), nil)
	if err != nil {
		return err
	}
	getReq.Header.Set("Authorization", "Bearer "+tok)
	getResp, err := a.http.Do(getReq)
	if err != nil {
		return err
	}
	defer func() { _ = getResp.Body.Close() }()
	rawUser, _ := io.ReadAll(io.LimitReader(getResp.Body, 1<<20))
	if getResp.StatusCode != http.StatusOK {
		return fmt.Errorf("get user: %d %s", getResp.StatusCode, string(rawUser))
	}
	var user map[string]any
	if err := json.Unmarshal(rawUser, &user); err != nil {
		return err
	}
	user["requiredActions"] = []string{}
	user["emailVerified"] = true
	raw, _ := json.Marshal(user)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		a.internalBase+"/admin/realms/kalke/users/"+url.PathEscape(userID), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("clear required actions: %d %s", resp.StatusCode, string(b))
	}
	return nil
}

type RealmUser struct {
	ID      string
	Email   string
	Enabled bool
}

// FindUserByEmail returns the first exact email match in the kalke realm.
func (a *AdminClient) FindUserByEmail(ctx context.Context, email string) (RealmUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return RealmUser{}, fmt.Errorf("email required")
	}
	tok, err := a.token(ctx)
	if err != nil {
		return RealmUser{}, err
	}
	q := url.Values{}
	q.Set("email", email)
	q.Set("exact", "true")
	q.Set("max", "2")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.internalBase+"/admin/realms/kalke/users?"+q.Encode(), nil)
	if err != nil {
		return RealmUser{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return RealmUser{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return RealmUser{}, fmt.Errorf("find user: %d %s", resp.StatusCode, string(body))
	}
	var users []struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return RealmUser{}, err
	}
	if len(users) == 0 {
		return RealmUser{}, fmt.Errorf("user not found")
	}
	u := users[0]
	if u.ID == "" {
		return RealmUser{}, fmt.Errorf("user not found")
	}
	outEmail := strings.ToLower(strings.TrimSpace(u.Email))
	if outEmail == "" {
		outEmail = email
	}
	return RealmUser{ID: u.ID, Email: outEmail, Enabled: u.Enabled}, nil
}

// ListRealmRoleNames returns effective realm role names for a user (composites expanded).
func (a *AdminClient) ListRealmRoleNames(ctx context.Context, userID string) ([]string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id required")
	}
	tok, err := a.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.internalBase+"/admin/realms/kalke/users/"+url.PathEscape(userID)+"/role-mappings/realm/composite", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list roles: %d %s", resp.StatusCode, string(body))
	}
	var roles []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		name := strings.TrimSpace(r.Name)
		if name == "" || isKeycloakBuiltinRole(name) {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

func isKeycloakBuiltinRole(name string) bool {
	if strings.HasPrefix(name, "default-roles-") {
		return true
	}
	switch name {
	case "offline_access", "uma_authorization", "manage-account", "manage-account-links", "view-profile":
		return true
	default:
		return false
	}
}

// SetPassword updates a user's password via the Admin API (non-temporary).
func (a *AdminClient) SetPassword(ctx context.Context, userID, password string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || password == "" {
		return fmt.Errorf("user id and password required")
	}
	tok, err := a.token(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"type":      "password",
		"value":     password,
		"temporary": false,
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		a.internalBase+"/admin/realms/kalke/users/"+url.PathEscape(userID)+"/reset-password",
		bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("set password: %d %s", resp.StatusCode, string(b))
	}
	return a.clearRequiredActions(ctx, tok, userID)
}

// CreateUserWithRole creates a user and assigns a non-privileged realm role.
func (a *AdminClient) CreateUserWithRole(ctx context.Context, email, password, roleName string) (userID string, err error) {
	switch strings.TrimSpace(roleName) {
	case "admin", "bank:write":
		return "", fmt.Errorf("role %q cannot be assigned via API", roleName)
	}
	userID, err = a.CreateUser(ctx, "", email, password)
	if err != nil {
		return "", err
	}
	tok, err := a.token(ctx)
	if err != nil {
		return userID, err
	}
	if err := a.assignRealmRole(ctx, tok, userID, roleName); err != nil {
		return userID, err
	}
	return userID, nil
}

func (a *AdminClient) assignRealmRole(ctx context.Context, tok, userID, roleName string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		a.internalBase+"/admin/realms/kalke/roles/"+url.PathEscape(roleName), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get role: %d", resp.StatusCode)
	}
	var role map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		return err
	}
	raw, _ := json.Marshal([]map[string]any{role})
	req2, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.internalBase+"/admin/realms/kalke/users/"+url.PathEscape(userID)+"/role-mappings/realm",
		bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req2.Header.Set("Authorization", "Bearer "+tok)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := a.http.Do(req2)
	if err != nil {
		return err
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusNoContent && resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("assign role: %d", resp2.StatusCode)
	}
	return nil
}
