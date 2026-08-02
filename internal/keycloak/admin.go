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
	Username      string              `json:"username"`
	Email         string              `json:"email"`
	FirstName     string              `json:"firstName,omitempty"`
	Enabled       bool                `json:"enabled"`
	EmailVerified bool                `json:"emailVerified"`
	Credentials   []map[string]any    `json:"credentials"`
	Attributes    map[string][]string `json:"attributes,omitempty"`
}

// CreateUser creates a realm user with password and no realm roles.
// Used after email OTP verification — never grant admin via public signup.
func (a *AdminClient) CreateUser(ctx context.Context, name, email, password string) (userID string, err error) {
	tok, err := a.token(ctx)
	if err != nil {
		return "", err
	}
	payload := createUserBody{
		Username:      email,
		Email:         email,
		FirstName:     strings.TrimSpace(name),
		Enabled:       true,
		EmailVerified: true,
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
	return parts[len(parts)-1], nil
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
