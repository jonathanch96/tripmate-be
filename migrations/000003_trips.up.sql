CREATE TABLE tripmate.trips (
    id UUID PRIMARY KEY, code VARCHAR(10) NOT NULL, name VARCHAR(160) NOT NULL,
    base_currency CHAR(3) NOT NULL, start_date DATE NOT NULL, end_date DATE NOT NULL,
    planner_id UUID NOT NULL REFERENCES tripmate.users(id), is_finalized BOOLEAN NOT NULL DEFAULT FALSE,
    finalized_at TIMESTAMPTZ, is_archived BOOLEAN NOT NULL DEFAULT FALSE, archived_at TIMESTAMPTZ,
    setting_edit_permission VARCHAR(20) NOT NULL DEFAULT 'everyone'
        CHECK (setting_edit_permission IN ('everyone','own_only')),
    setting_approval_expenses BOOLEAN NOT NULL DEFAULT FALSE,
    setting_approval_settlements BOOLEAN NOT NULL DEFAULT TRUE,
    setting_multi_currency_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    setting_allow_settlement_before_end BOOLEAN NOT NULL DEFAULT TRUE,
    version INT NOT NULL DEFAULT 1, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), deleted_at TIMESTAMPTZ,
    CHECK (end_date >= start_date)
);
CREATE UNIQUE INDEX ux_trips_code ON tripmate.trips (code) WHERE deleted_at IS NULL;
CREATE INDEX ix_trips_planner ON tripmate.trips (planner_id);
CREATE INDEX ix_trips_planner_archived ON tripmate.trips (planner_id, is_archived);

CREATE TABLE tripmate.trip_participants (
    id UUID PRIMARY KEY, trip_id UUID NOT NULL REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES tripmate.users(id), role VARCHAR(20) NOT NULL DEFAULT 'participant'
        CHECK (role IN ('planner','participant')),
    bank_name VARCHAR(120), bank_account_number VARCHAR(64), bank_account_holder VARCHAR(160),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX ux_trip_participants ON tripmate.trip_participants (trip_id, user_id) WHERE deleted_at IS NULL;
CREATE INDEX ix_trip_participants_user ON tripmate.trip_participants (user_id) WHERE deleted_at IS NULL;

CREATE TABLE tripmate.trip_invitations (
    id UUID PRIMARY KEY, trip_id UUID NOT NULL REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    email CITEXT NOT NULL, token VARCHAR(64) NOT NULL, status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','accepted','revoked','expired')),
    invited_by UUID NOT NULL REFERENCES tripmate.users(id), expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_trip_invitations_token ON tripmate.trip_invitations (token);
CREATE UNIQUE INDEX ux_trip_invitations_pending ON tripmate.trip_invitations (trip_id, email) WHERE status = 'pending';
CREATE INDEX ix_trip_invitations_email ON tripmate.trip_invitations (email) WHERE status = 'pending';
