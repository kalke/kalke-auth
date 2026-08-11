package otp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kalke/kalke-auth/internal/security"
)

// PasswordPending is an email-OTP gated password change.
type PasswordPending struct {
	UserSub      string    `json:"user_sub"`
	Email        string    `json:"email"`
	OTPHash      string    `json:"otp_hash"`
	PasswordSeal string    `json:"password_seal"`
	CreatedAt    time.Time `json:"created_at"`
	LastSentAt   time.Time `json:"last_sent_at"`
	OTPExpiresAt time.Time `json:"otp_expires_at"`
	Attempts     int       `json:"attempts"`
}

type PasswordStore struct {
	rdb    *redis.Client
	pepper []byte
	prefix string
}

func NewPasswordStore(rdb *redis.Client, pepper []byte) *PasswordStore {
	return &PasswordStore{rdb: rdb, pepper: pepper, prefix: "pwd:otp:"}
}

func (s *PasswordStore) key(userSub string) string {
	return s.prefix + strings.TrimSpace(userSub)
}

func (s *PasswordStore) Put(ctx context.Context, p PasswordPending) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key(p.UserSub), raw, ChallengeTTL).Err()
}

func (s *PasswordStore) Get(ctx context.Context, userSub string) (PasswordPending, error) {
	raw, err := s.rdb.Get(ctx, s.key(userSub)).Bytes()
	if err == redis.Nil {
		return PasswordPending{}, ErrNotFound
	}
	if err != nil {
		return PasswordPending{}, err
	}
	var p PasswordPending
	if err := json.Unmarshal(raw, &p); err != nil {
		return PasswordPending{}, err
	}
	return p, nil
}

func (s *PasswordStore) Delete(ctx context.Context, userSub string) error {
	return s.rdb.Del(ctx, s.key(userSub)).Err()
}

func (s *PasswordStore) New(userSub, email, newPassword string) (PasswordPending, string, error) {
	code, err := RandomOTP()
	if err != nil {
		return PasswordPending{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, code)
	if err != nil {
		return PasswordPending{}, "", err
	}
	seal, err := security.Seal(s.pepper, newPassword)
	if err != nil {
		return PasswordPending{}, "", err
	}
	now := time.Now().UTC()
	p := PasswordPending{
		UserSub:      strings.TrimSpace(userSub),
		Email:        strings.ToLower(strings.TrimSpace(email)),
		OTPHash:      otpHash,
		PasswordSeal: seal,
		CreatedAt:    now,
		LastSentAt:   now,
		OTPExpiresAt: now.Add(OTPTTL),
		Attempts:     0,
	}
	return p, code, nil
}

func (s *PasswordStore) Rotate(p PasswordPending) (PasswordPending, string, error) {
	code, err := RandomOTP()
	if err != nil {
		return PasswordPending{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, code)
	if err != nil {
		return PasswordPending{}, "", err
	}
	now := time.Now().UTC()
	p.OTPHash = otpHash
	p.LastSentAt = now
	p.OTPExpiresAt = now.Add(OTPTTL)
	p.Attempts = 0
	return p, code, nil
}

func (s *PasswordStore) ResendAllowed(p PasswordPending) (time.Duration, bool) {
	wait := ResendCooldown - time.Since(p.LastSentAt)
	if wait > 0 {
		return wait, false
	}
	return 0, true
}

func (s *PasswordStore) Verify(p *PasswordPending, code string) error {
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

func (s *PasswordStore) OpenPassword(p PasswordPending) (string, error) {
	return security.Open(s.pepper, p.PasswordSeal)
}
