package otp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kalke/kalke-auth/internal/security"
)

const (
	ChallengeTTL      = 30 * time.Minute
	OTPTTL            = 15 * time.Minute
	ResendCooldown    = 2 * time.Minute
	MaxVerifyAttempts = 5
)

var (
	ErrNotFound     = errors.New("otp challenge not found")
	ErrOTPExpired   = errors.New("otp expired")
	ErrTooManyTries = errors.New("too many attempts")
	ErrInvalidOTP   = errors.New("invalid otp")
)

type Challenge struct {
	Email        string    `json:"email"`
	UserSub      string    `json:"user_sub,omitempty"` // email-change: owner of the pending request
	OTPHash      string    `json:"otp_hash"`
	CreatedAt    time.Time `json:"created_at"`
	LastSentAt   time.Time `json:"last_sent_at"`
	OTPExpiresAt time.Time `json:"otp_expires_at"`
	Attempts     int       `json:"attempts"`
}

type Store struct {
	rdb    *redis.Client
	pepper []byte
	prefix string
}

func NewStore(rdb *redis.Client, pepper []byte, prefix string) *Store {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "otp:"
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &Store{rdb: rdb, pepper: pepper, prefix: prefix}
}

func (s *Store) key(email string) string {
	return s.prefix + strings.ToLower(strings.TrimSpace(email))
}

func RandomOTP() (string, error) {
	max := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *Store) Put(ctx context.Context, c Challenge) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key(c.Email), raw, ChallengeTTL).Err()
}

func (s *Store) Get(ctx context.Context, email string) (Challenge, error) {
	raw, err := s.rdb.Get(ctx, s.key(email)).Bytes()
	if err == redis.Nil {
		return Challenge{}, ErrNotFound
	}
	if err != nil {
		return Challenge{}, err
	}
	var c Challenge
	if err := json.Unmarshal(raw, &c); err != nil {
		return Challenge{}, err
	}
	return c, nil
}

func (s *Store) Delete(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, s.key(email)).Err()
}

func (s *Store) New(email string) (Challenge, string, error) {
	otp, err := RandomOTP()
	if err != nil {
		return Challenge{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, otp)
	if err != nil {
		return Challenge{}, "", err
	}
	now := time.Now().UTC()
	c := Challenge{
		Email:        strings.ToLower(strings.TrimSpace(email)),
		OTPHash:      otpHash,
		CreatedAt:    now,
		LastSentAt:   now,
		OTPExpiresAt: now.Add(OTPTTL),
		Attempts:     0,
	}
	return c, otp, nil
}

func (s *Store) Rotate(c Challenge) (Challenge, string, error) {
	otp, err := RandomOTP()
	if err != nil {
		return Challenge{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, otp)
	if err != nil {
		return Challenge{}, "", err
	}
	now := time.Now().UTC()
	c.OTPHash = otpHash
	c.LastSentAt = now
	c.OTPExpiresAt = now.Add(OTPTTL)
	c.Attempts = 0
	return c, otp, nil
}

func (s *Store) ResendAllowed(c Challenge) (time.Duration, bool) {
	wait := ResendCooldown - time.Since(c.LastSentAt)
	if wait > 0 {
		return wait, false
	}
	return 0, true
}

func (s *Store) Verify(c *Challenge, code string) error {
	if c.Attempts >= MaxVerifyAttempts {
		return ErrTooManyTries
	}
	if time.Now().UTC().After(c.OTPExpiresAt) {
		return ErrOTPExpired
	}
	c.Attempts++
	if !security.CheckSecret(s.pepper, strings.TrimSpace(code), c.OTPHash) {
		return ErrInvalidOTP
	}
	return nil
}

func SecondsUntil(t time.Time) int {
	d := time.Until(t)
	if d <= 0 {
		return 0
	}
	sec := int(d.Seconds())
	if sec < 1 {
		return 1
	}
	return sec
}
