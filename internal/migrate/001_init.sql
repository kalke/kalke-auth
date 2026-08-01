CREATE TABLE IF NOT EXISTS sessions (
	id UUID PRIMARY KEY,
	user_sub TEXT NOT NULL,
	user_email TEXT NOT NULL,
	permissions TEXT[] NOT NULL DEFAULT '{}',
	token_hash TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sessions_user_sub_idx ON sessions (user_sub);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE IF NOT EXISTS api_tokens (
	id UUID PRIMARY KEY,
	user_sub TEXT NOT NULL,
	user_email TEXT NOT NULL,
	name TEXT NOT NULL,
	token_prefix TEXT NOT NULL,
	token_hash TEXT NOT NULL,
	permissions TEXT[] NOT NULL DEFAULT '{}',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	last_used_at TIMESTAMPTZ,
	revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS api_tokens_user_sub_idx ON api_tokens (user_sub);
CREATE UNIQUE INDEX IF NOT EXISTS api_tokens_prefix_uidx ON api_tokens (token_prefix);
