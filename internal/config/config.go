package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	RedisAddr          string
	RedisPassword      string
	RedisTLS           bool
	SessionSecret      []byte
	TokenPepper        []byte
	IntrospectSecret   string
	KCInternalURL      string
	KCPublicIssuer     string
	BFFClientID        string
	BFFClientSecret    string
	CORSOrigins        []string
	CookieDomain       string
	SessionTTL         time.Duration
	LoginRatePerMinute int
}

func Load() (Config, error) {
	sessionTTL, err := time.ParseDuration(getenv("SESSION_TTL", "24h"))
	if err != nil || sessionTTL <= 0 {
		return Config{}, fmt.Errorf("invalid SESSION_TTL")
	}
	loginRate, err := strconv.Atoi(getenv("LOGIN_RATE_PER_MINUTE", "10"))
	if err != nil || loginRate <= 0 {
		return Config{}, fmt.Errorf("invalid LOGIN_RATE_PER_MINUTE")
	}

	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	tokenPepper := strings.TrimSpace(os.Getenv("TOKEN_HASH_PEPPER"))
	introspect := strings.TrimSpace(os.Getenv("INTROSPECT_SECRET"))
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	bffID := getenv("KC_BFF_CLIENT_ID", "kalke-bff")
	bffSecret := strings.TrimSpace(os.Getenv("KC_BFF_CLIENT_SECRET"))
	if dbURL == "" || sessionSecret == "" || tokenPepper == "" || introspect == "" || bffSecret == "" {
		return Config{}, fmt.Errorf("DATABASE_URL, SESSION_SECRET, TOKEN_HASH_PEPPER, INTROSPECT_SECRET, KC_BFF_CLIENT_SECRET are required")
	}

	cors := parseCSV(os.Getenv("CORS_ORIGINS"))
	if len(cors) == 0 {
		cors = []string{"https://kalke.dev", "https://www.kalke.dev"}
	}

	return Config{
		HTTPAddr:           getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:        dbURL,
		RedisAddr:          getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      os.Getenv("REDIS_PASSWORD"),
		RedisTLS:           parseBool(os.Getenv("REDIS_TLS"), true),
		SessionSecret:      []byte(sessionSecret),
		TokenPepper:        []byte(tokenPepper),
		IntrospectSecret:   introspect,
		KCInternalURL:      strings.TrimSuffix(getenv("KC_INTERNAL_URL", "http://127.0.0.1:8081"), "/"),
		KCPublicIssuer:     strings.TrimSuffix(getenv("KC_PUBLIC_ISSUER", "https://auth.kalke.dev/realms/kalke"), "/"),
		BFFClientID:        bffID,
		BFFClientSecret:    bffSecret,
		CORSOrigins:        cors,
		CookieDomain:       getenv("COOKIE_DOMAIN", ".kalke.dev"),
		SessionTTL:         sessionTTL,
		LoginRatePerMinute: loginRate,
	}, nil
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func parseCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
