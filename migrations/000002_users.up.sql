CREATE TABLE tripmate.users (
    id            UUID PRIMARY KEY,
    email         CITEXT NOT NULL,
    name          VARCHAR(120) NOT NULL,
    password_hash TEXT NOT NULL,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX ux_users_email ON tripmate.users (email) WHERE deleted_at IS NULL;

CREATE TABLE tripmate.refresh_tokens (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES tripmate.users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    user_agent TEXT,
    ip         INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_refresh_tokens_hash ON tripmate.refresh_tokens (token_hash);
CREATE INDEX ix_refresh_tokens_user ON tripmate.refresh_tokens (user_id) WHERE revoked_at IS NULL;
