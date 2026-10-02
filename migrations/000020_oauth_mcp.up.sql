-- OAuth 2.1 authorization server for the MCP endpoint: AI tools (Claude, ChatGPT, ...) register as
-- clients, a user approves them on the consent page, and the resulting grant carries the tokens the
-- tool presents to /mcp. Tokens are only ever stored as SHA-256 hashes.
CREATE TABLE tripmate.oauth_clients (
    id                         UUID PRIMARY KEY,
    client_id                  VARCHAR(64) NOT NULL UNIQUE,
    client_secret_hash         TEXT,
    name                       VARCHAR(120) NOT NULL,
    client_uri                 TEXT,
    logo_uri                   TEXT,
    redirect_uris              TEXT[] NOT NULL,
    token_endpoint_auth_method VARCHAR(30) NOT NULL DEFAULT 'none'
        CHECK (token_endpoint_auth_method IN ('none','client_secret_post','client_secret_basic')),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per /oauth/authorize visit. Approving it on the consent page mints the authorization
-- code onto the same row; exchanging the code marks it used.
CREATE TABLE tripmate.oauth_auth_requests (
    id                    UUID PRIMARY KEY,
    client_id             UUID NOT NULL REFERENCES tripmate.oauth_clients(id) ON DELETE CASCADE,
    redirect_uri          TEXT NOT NULL,
    scopes                TEXT[] NOT NULL,
    state                 TEXT,
    code_challenge        TEXT NOT NULL,
    resource              TEXT,
    status                VARCHAR(10) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','approved','denied','used')),
    user_id               UUID REFERENCES tripmate.users(id) ON DELETE CASCADE,
    granted_scopes        TEXT[],
    code_hash             TEXT UNIQUE,
    code_expires_at       TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_oauth_auth_requests_client ON tripmate.oauth_auth_requests (client_id);
CREATE INDEX ix_oauth_auth_requests_user ON tripmate.oauth_auth_requests (user_id);

-- A grant is one "connected app": one per user and client while active.
CREATE TABLE tripmate.oauth_grants (
    id           UUID PRIMARY KEY,
    client_id    UUID NOT NULL REFERENCES tripmate.oauth_clients(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES tripmate.users(id) ON DELETE CASCADE,
    scopes       TEXT[] NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX ux_oauth_grants_active ON tripmate.oauth_grants (user_id, client_id) WHERE revoked_at IS NULL;
CREATE INDEX ix_oauth_grants_client ON tripmate.oauth_grants (client_id);

CREATE TABLE tripmate.oauth_tokens (
    id          UUID PRIMARY KEY,
    grant_id    UUID NOT NULL REFERENCES tripmate.oauth_grants(id) ON DELETE CASCADE,
    kind        VARCHAR(10) NOT NULL CHECK (kind IN ('access','refresh')),
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_oauth_tokens_grant ON tripmate.oauth_tokens (grant_id) WHERE revoked_at IS NULL;

-- Expenses created by a connected AI tool record which one, so the ledger can say "via Claude".
ALTER TABLE tripmate.expenses DROP CONSTRAINT IF EXISTS expenses_source_check;
ALTER TABLE tripmate.expenses ADD CONSTRAINT expenses_source_check CHECK (source IN ('manual','receipt','assistant'));
ALTER TABLE tripmate.expenses ADD COLUMN created_via VARCHAR(120);
