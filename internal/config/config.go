package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr            string
	DatabaseURL         string
	DBSearchPath        string
	RedisAddr           string
	RedisPassword       string
	RedisTLS            bool
	SessionSecret       []byte
	TokenPepper         []byte
	IntrospectSecret    string
	SignupEnabled       bool
	AdminEmails         []string
	MailFrom            string
	MailgunAPIKey       string
	MailgunDomain       string
	ResendAPIKey        string
	MailDevLog          bool
	KCInternalURL       string
	KCPublicIssuer      string
	BFFClientID         string
	BFFClientSecret     string
	KCAdminUser         string
	KCAdminPassword     string
	CORSOrigins         []string
	CookieDomain        string
	SessionTTL          time.Duration
	LoginRatePerMinute  int
	SignupRatePerMinute int
	SignupRatePerHour   int
	OAuthRedirectURI    string
	OAuthSuccessURL     string
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
	signupRateMin, err := strconv.Atoi(getenv("SIGNUP_RATE_PER_MINUTE", "3"))
	if err != nil || signupRateMin <= 0 {
		return Config{}, fmt.Errorf("invalid SIGNUP_RATE_PER_MINUTE")
	}
	signupRateHour, err := strconv.Atoi(getenv("SIGNUP_RATE_PER_HOUR", "10"))
	if err != nil || signupRateHour <= 0 {
		return Config{}, fmt.Errorf("invalid SIGNUP_RATE_PER_HOUR")
	}

	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	tokenPepper := strings.TrimSpace(os.Getenv("TOKEN_HASH_PEPPER"))
	introspect := strings.TrimSpace(os.Getenv("INTROSPECT_SECRET"))
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	bffID := getenv("KC_BFF_CLIENT_ID", "kalke-bff")
	bffSecret := strings.TrimSpace(os.Getenv("KC_BFF_CLIENT_SECRET"))
	adminUser := strings.TrimSpace(os.Getenv("KC_BOOTSTRAP_ADMIN_USERNAME"))
	adminPass := strings.TrimSpace(os.Getenv("KC_BOOTSTRAP_ADMIN_PASSWORD"))
	if dbURL == "" || sessionSecret == "" || tokenPepper == "" || introspect == "" || bffSecret == "" {
		return Config{}, fmt.Errorf("DATABASE_URL, SESSION_SECRET, TOKEN_HASH_PEPPER, INTROSPECT_SECRET, KC_BFF_CLIENT_SECRET are required")
	}
	if adminUser == "" || adminPass == "" {
		return Config{}, fmt.Errorf("KC_BOOTSTRAP_ADMIN_USERNAME, KC_BOOTSTRAP_ADMIN_PASSWORD are required")
	}

	signupEnabled := parseBool(getenv("SIGNUP_ENABLED", "true"), true)
	mailDevLog := parseBool(os.Getenv("MAIL_DEV_LOG"), false)
	mailgunKey := strings.TrimSpace(os.Getenv("MAILGUN_API_KEY"))
	mailgunDomain := strings.TrimSpace(os.Getenv("MAILGUN_DOMAIN"))
	resendKey := strings.TrimSpace(os.Getenv("RESEND_API_KEY"))
	mailFrom := getenv("MAIL_FROM", "kalke <noreply@kalke.dev>")
	if signupEnabled && !mailDevLog && mailgunKey == "" && resendKey == "" {
		return Config{}, fmt.Errorf("MAILGUN_API_KEY or RESEND_API_KEY is required when SIGNUP_ENABLED=true (or set MAIL_DEV_LOG=true)")
	}
	if signupEnabled && mailgunKey != "" && mailgunDomain == "" {
		return Config{}, fmt.Errorf("MAILGUN_DOMAIN is required when MAILGUN_API_KEY is set")
	}

	cors := parseCSV(os.Getenv("CORS_ORIGINS"))
	if len(cors) == 0 {
		cors = []string{"https://kalke.dev", "https://www.kalke.dev"}
	}
	adminEmails := parseCSV(os.Getenv("ADMIN_EMAILS"))
	if len(adminEmails) == 0 {
		adminEmails = []string{"henriquekalke@icloud.com"}
	}

	return Config{
		HTTPAddr:            getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:         dbURL,
		DBSearchPath:        getenv("DB_SEARCH_PATH", "app"),
		RedisAddr:           getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       os.Getenv("REDIS_PASSWORD"),
		RedisTLS:            parseBool(os.Getenv("REDIS_TLS"), true),
		SessionSecret:       []byte(sessionSecret),
		TokenPepper:         []byte(tokenPepper),
		IntrospectSecret:    introspect,
		SignupEnabled:       signupEnabled,
		AdminEmails:         adminEmails,
		MailFrom:            mailFrom,
		MailgunAPIKey:       mailgunKey,
		MailgunDomain:       mailgunDomain,
		ResendAPIKey:        resendKey,
		MailDevLog:          mailDevLog,
		KCInternalURL:       strings.TrimSuffix(getenv("KC_INTERNAL_URL", "http://127.0.0.1:8081"), "/"),
		KCPublicIssuer:      strings.TrimSuffix(getenv("KC_PUBLIC_ISSUER", "https://auth.kalke.dev/realms/kalke"), "/"),
		BFFClientID:         bffID,
		BFFClientSecret:     bffSecret,
		KCAdminUser:         adminUser,
		KCAdminPassword:     adminPass,
		CORSOrigins:         cors,
		CookieDomain:        getenv("COOKIE_DOMAIN", ".kalke.dev"),
		SessionTTL:          sessionTTL,
		LoginRatePerMinute:  loginRate,
		SignupRatePerMinute: signupRateMin,
		SignupRatePerHour:   signupRateHour,
		OAuthRedirectURI:    getenv("OAUTH_REDIRECT_URI", "https://auth.kalke.dev/v1/auth/callback"),
		OAuthSuccessURL:     getenv("OAUTH_SUCCESS_URL", "https://kalke.dev/playground"),
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
