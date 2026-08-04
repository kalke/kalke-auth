package keycloak

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	internalBase string
	publicIssuer string
	clientID     string
	clientSecret string
	http         *http.Client
}

func New(internalBase, publicIssuer, clientID, clientSecret string) *Client {
	return &Client{
		internalBase: strings.TrimSuffix(internalBase, "/"),
		publicIssuer: strings.TrimSuffix(publicIssuer, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 20 * time.Second},
	}
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type accessClaims struct {
	Sub         string   `json:"sub"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
	Scope       string   `json:"scope"`
	Iss         string   `json:"iss"`
	Aud         any      `json:"aud"`
}

type UserInfo struct {
	Subject     string
	Email       string
	Permissions []string
}

func (c *Client) PasswordLogin(ctx context.Context, username, password string) (UserInfo, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("username", username)
	form.Set("password", password)
	form.Set("scope", "openid profile email")
	return c.tokenUserInfo(ctx, form)
}

// AuthorizationURL builds the browser redirect into Keycloak (optionally with IdP hint).
func (c *Client) AuthorizationURL(redirectURI, state, codeChallenge, idpHint string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("response_type", "code")
	q.Set("scope", "openid profile email")
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	if idpHint != "" {
		q.Set("kc_idp_hint", idpHint)
	}
	return c.publicIssuer + "/protocol/openid-connect/auth?" + q.Encode()
}

// ExchangeCode swaps an authorization code (+ PKCE verifier) for user claims.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (UserInfo, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	return c.tokenUserInfo(ctx, form)
}

func (c *Client) tokenUserInfo(ctx context.Context, form url.Values) (UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.internalBase+"/realms/kalke/protocol/openid-connect/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return UserInfo{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return UserInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, fmt.Errorf("keycloak token failed: %d", resp.StatusCode)
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return UserInfo{}, err
	}
	claims, err := decodeAccessClaims(tr.AccessToken)
	if err != nil {
		return UserInfo{}, err
	}
	if claims.Sub == "" {
		return UserInfo{}, fmt.Errorf("missing sub")
	}
	perms := claims.Permissions
	if len(perms) == 0 && claims.Scope != "" {
		perms = strings.Fields(claims.Scope)
	}
	return UserInfo{
		Subject:     claims.Sub,
		Email:       claims.Email,
		Permissions: perms,
	}, nil
}

// ClientCredentialsToken fetches an access token for a confidential M2M client.
func (c *Client) ClientCredentialsToken(ctx context.Context, clientID, clientSecret string) (string, time.Duration, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.internalBase+"/realms/kalke/protocol/openid-connect/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("keycloak client_credentials failed: %d", resp.StatusCode)
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", 0, err
	}
	if tr.AccessToken == "" {
		return "", 0, fmt.Errorf("empty access_token")
	}
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return tr.AccessToken, ttl, nil
}

func decodeAccessClaims(jwt string) (accessClaims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return accessClaims{}, fmt.Errorf("invalid jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// some libs use std encoding with padding
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return accessClaims{}, err
		}
	}
	var c accessClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return accessClaims{}, err
	}
	return c, nil
}

func (c *Client) PublicIssuer() string { return c.publicIssuer }
