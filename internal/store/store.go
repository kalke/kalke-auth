package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Session struct {
	ID          uuid.UUID
	UserSub     string
	UserEmail   string
	Permissions []string
	TokenHash   string
	ExpiresAt   time.Time
}

type APIToken struct {
	ID          uuid.UUID
	UserSub     string
	UserEmail   string
	Name        string
	TokenPrefix string
	TokenHash   string
	Permissions []string
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_sub, user_email, permissions, token_hash, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		sess.ID, sess.UserSub, sess.UserEmail, sess.Permissions, sess.TokenHash, sess.ExpiresAt,
	)
	return err
}

func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_sub, user_email, permissions, token_hash, expires_at
		FROM sessions WHERE id = $1 AND expires_at > now()`, id).Scan(
		&sess.ID, &sess.UserSub, &sess.UserEmail, &sess.Permissions, &sess.TokenHash, &sess.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return sess, err
}

func (s *Store) DeleteSession(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

func (s *Store) CreateAPIToken(ctx context.Context, t APIToken) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_tokens (id, user_sub, user_email, name, token_prefix, token_hash, permissions)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		t.ID, t.UserSub, t.UserEmail, t.Name, t.TokenPrefix, t.TokenHash, t.Permissions,
	)
	return err
}

func (s *Store) ListAPITokens(ctx context.Context, userSub string) ([]APIToken, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_sub, user_email, name, token_prefix, token_hash, permissions, created_at, last_used_at, revoked_at
		FROM api_tokens
		WHERE user_sub = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC`, userSub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		if err := rows.Scan(&t.ID, &t.UserSub, &t.UserEmail, &t.Name, &t.TokenPrefix, &t.TokenHash, &t.Permissions, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIToken(ctx context.Context, id uuid.UUID, userSub string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE api_tokens SET revoked_at = now()
		WHERE id = $1 AND user_sub = $2 AND revoked_at IS NULL`, id, userSub)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAPITokenByPrefix(ctx context.Context, prefix string) (APIToken, error) {
	var t APIToken
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_sub, user_email, name, token_prefix, token_hash, permissions, created_at, last_used_at, revoked_at
		FROM api_tokens WHERE token_prefix = $1 AND revoked_at IS NULL`, prefix).Scan(
		&t.ID, &t.UserSub, &t.UserEmail, &t.Name, &t.TokenPrefix, &t.TokenHash, &t.Permissions, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIToken{}, ErrNotFound
	}
	return t, err
}

func (s *Store) TouchAPIToken(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE id = $1`, id)
	return err
}
