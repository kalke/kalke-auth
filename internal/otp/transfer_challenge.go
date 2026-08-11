package otp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kalke/kalke-auth/internal/security"
)

// TransferPending is an email-OTP gated bank transfer intent.
type TransferPending struct {
	UserSub              string    `json:"user_sub"`
	Email                string    `json:"email"`
	OTPHash              string    `json:"otp_hash"`
	CreatedAt            time.Time `json:"created_at"`
	LastSentAt           time.Time `json:"last_sent_at"`
	OTPExpiresAt         time.Time `json:"otp_expires_at"`
	Attempts             int       `json:"attempts"`
	Amount               string    `json:"amount"`
	DestinationAccount   string    `json:"destination_account,omitempty"`
	DestinationAccountID string    `json:"destination_account_id,omitempty"`
	DestinationDocument  string    `json:"destination_document,omitempty"`
	DestinationHolder    string    `json:"destination_holder,omitempty"`
	DestinationDisplay   string    `json:"destination_display,omitempty"`
	Memo                 string    `json:"memo,omitempty"`
	SourceAccountID      string    `json:"source_account_id,omitempty"`
	IdempotencyKey       string    `json:"idempotency_key"`
}

type TransferStore struct {
	rdb    *redis.Client
	pepper []byte
	prefix string
}

func NewTransferStore(rdb *redis.Client, pepper []byte) *TransferStore {
	return &TransferStore{rdb: rdb, pepper: pepper, prefix: "xfer:otp:"}
}

func (s *TransferStore) key(userSub string) string {
	return s.prefix + strings.TrimSpace(userSub)
}

func (s *TransferStore) Put(ctx context.Context, p TransferPending) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.key(p.UserSub), raw, ChallengeTTL).Err()
}

func (s *TransferStore) Get(ctx context.Context, userSub string) (TransferPending, error) {
	raw, err := s.rdb.Get(ctx, s.key(userSub)).Bytes()
	if err == redis.Nil {
		return TransferPending{}, ErrNotFound
	}
	if err != nil {
		return TransferPending{}, err
	}
	var p TransferPending
	if err := json.Unmarshal(raw, &p); err != nil {
		return TransferPending{}, err
	}
	return p, nil
}

func (s *TransferStore) Delete(ctx context.Context, userSub string) error {
	return s.rdb.Del(ctx, s.key(userSub)).Err()
}

func (s *TransferStore) New(
	userSub, email, amount, destAccount, destAccountID, destDocument, destHolder, destDisplay, memo, sourceAccountID, idemKey string,
) (TransferPending, string, error) {
	code, err := RandomOTP()
	if err != nil {
		return TransferPending{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, code)
	if err != nil {
		return TransferPending{}, "", err
	}
	now := time.Now().UTC()
	p := TransferPending{
		UserSub:              strings.TrimSpace(userSub),
		Email:                strings.ToLower(strings.TrimSpace(email)),
		OTPHash:              otpHash,
		CreatedAt:            now,
		LastSentAt:           now,
		OTPExpiresAt:         now.Add(OTPTTL),
		Attempts:             0,
		Amount:               strings.TrimSpace(amount),
		DestinationAccount:   strings.TrimSpace(destAccount),
		DestinationAccountID: strings.TrimSpace(destAccountID),
		DestinationDocument:  strings.TrimSpace(destDocument),
		DestinationHolder:    strings.TrimSpace(destHolder),
		DestinationDisplay:   strings.TrimSpace(destDisplay),
		Memo:                 strings.TrimSpace(memo),
		SourceAccountID:      strings.TrimSpace(sourceAccountID),
		IdempotencyKey:       strings.TrimSpace(idemKey),
	}
	return p, code, nil
}

func (s *TransferStore) Rotate(p TransferPending) (TransferPending, string, error) {
	code, err := RandomOTP()
	if err != nil {
		return TransferPending{}, "", err
	}
	otpHash, err := security.HashSecret(s.pepper, code)
	if err != nil {
		return TransferPending{}, "", err
	}
	now := time.Now().UTC()
	p.OTPHash = otpHash
	p.LastSentAt = now
	p.OTPExpiresAt = now.Add(OTPTTL)
	p.Attempts = 0
	return p, code, nil
}

func (s *TransferStore) ResendAllowed(p TransferPending) (time.Duration, bool) {
	wait := ResendCooldown - time.Since(p.LastSentAt)
	if wait > 0 {
		return wait, false
	}
	return 0, true
}

func (s *TransferStore) Verify(p *TransferPending, code string) error {
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
