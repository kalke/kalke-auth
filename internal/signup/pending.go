package signup

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
	PendingTTL        = 30 * time.Minute
	OTPTTL            = 15 * time.Minute
	ResendCooldown     = 2 * time.Minute
	MaxVerifyAttempts = 5
)

var (
	ErrNotFound      = errors.New("pending signup not found")
	ErrResendTooSoon = errors.New("resend too soon")
	ErrOTPExpired    = errors.New("otp expired")
	ErrTooManyTries  = errors.New("too many attempts")
	ErrInvalidOTP    = errors.New("invalid otp")
)

type Pending struct {
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordSeal string    `json:"password_seal"`
	OTPHash      string    `json:"otp_hash"`
	CreatedAt    time.Time `json:"created_at"`
	LastSentAt   time.Time `json:"last_sent_at"`
	OTPExpiresAt time.Time `json:"otp_expires_at"`
	Attempts     int       `json:"attempts"`
}

type Store struct {
	rdb    *redis.Client
	pepper []byte
}

func NewStore(rdb *redis.Client, pepper []byte) *Store {
	return &Store{rdb: rdb, pepper: pepper}
}

func (s *Store) key(email string) string {
	return "signup:pending:" + strings.ToLower(strings.TrimSpace(email))
}

func RandomOTP() (string, error) {
	max := big.NewInt(1_000_000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *Store) Put(ctx context.Context, p Pending) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key(p.Email), raw, PendingTTL).Err()
}

func (s *Store) Get(ctx context.Context, email string) (Pending, error) {
	raw, err := s.rdb.Get(ctx, s.key(email)).Bytes()
	if err == redis.Nil {
		return Pending{}, ErrNotFound
	}
	if err != nil {
		return Pending{}, err
	}
	var p Pending
	if err := json.Unmarshal(raw, &p); err != nil {
		return Pending{}, err
	}
	return p, nil
}

func (s *Store) Delete(ctx context.Context, email string) error {
	return s.rdb.Del(ctx, s.key(email)).Err()
}

func (s *Store) NewPending(name, email, password string) (Pending, string, error) {
	seal, err := security.Seal(s.pepper, password)
	if err != nil {
		return Pending{}, "", err
	}
	otp, err := RandomOTP()
	if err != nil {
		return Pending{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, otp)
	if err != nil {
		return Pending{}, "", err
	}
	now := time.Now().UTC()
	p := Pending{
		Name:         strings.TrimSpace(name),
		Email:        strings.ToLower(strings.TrimSpace(email)),
		PasswordSeal: seal,
		OTPHash:      otpHash,
		CreatedAt:    now,
		LastSentAt:   now,
		OTPExpiresAt: now.Add(OTPTTL),
		Attempts:     0,
	}
	return p, otp, nil
}

func (s *Store) RotateOTP(p Pending) (Pending, string, error) {
	otp, err := RandomOTP()
	if err != nil {
		return Pending{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, otp)
	if err != nil {
		return Pending{}, "", err
	}
	now := time.Now().UTC()
	p.OTPHash = otpHash
	p.LastSentAt = now
	p.OTPExpiresAt = now.Add(OTPTTL)
	p.Attempts = 0
	return p, otp, nil
}

func (s *Store) ResendAllowed(p Pending) (time.Duration, bool) {
	wait := ResendCooldown - time.Since(p.LastSentAt)
	if wait > 0 {
		return wait, false
	}
	return 0, true
}

func (s *Store) VerifyOTP(p *Pending, code string) error {
	if p.Attempts >= MaxVerifyAttempts {
		return ErrTooManyTries
	}
	if time.Now().UTC().After(p.OTPExpiresAt) {
		return ErrOTPExpired
	}
	p.Attempts++
	if !security.CheckSecret(s.pepper, strings.TrimSpace(code), p.OTPHash) {
		return ErrInvalidOTP
	}
	return nil
}

func (s *Store) Password(p Pending) (string, error) {
	return security.Open(s.pepper, p.PasswordSeal)
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
