CREATE TABLE tripmate.expenses (
    id UUID PRIMARY KEY,
    trip_id UUID NOT NULL REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    expense_date DATE NOT NULL,
    description VARCHAR(255) NOT NULL,
    amount NUMERIC(20,6) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    split_type VARCHAR(10) NOT NULL CHECK (split_type IN ('equal','manual','item')),
    status VARCHAR(10) NOT NULL DEFAULT 'approved' CHECK (status IN ('pending','approved','rejected')),
    source VARCHAR(10) NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','receipt')),
    note TEXT,
    created_by_user_id UUID NOT NULL REFERENCES tripmate.users(id),
    approved_by_user_id UUID REFERENCES tripmate.users(id),
    approved_at TIMESTAMPTZ,
    rejected_reason TEXT,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX ix_expenses_trip_status ON tripmate.expenses (trip_id, status) WHERE deleted_at IS NULL;
CREATE INDEX ix_expenses_trip_date ON tripmate.expenses (trip_id, expense_date DESC) WHERE deleted_at IS NULL;

CREATE TABLE tripmate.expense_payers (
    id UUID PRIMARY KEY,
    expense_id UUID NOT NULL REFERENCES tripmate.expenses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES tripmate.users(id),
    amount NUMERIC(20,6) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_expense_payers ON tripmate.expense_payers (expense_id, user_id);
CREATE INDEX ix_expense_payers_user ON tripmate.expense_payers (user_id);

CREATE TABLE tripmate.expense_splits (
    id UUID PRIMARY KEY,
    expense_id UUID NOT NULL REFERENCES tripmate.expenses(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES tripmate.users(id),
    amount NUMERIC(20,6) NOT NULL CHECK (amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_expense_splits ON tripmate.expense_splits (expense_id, user_id);
CREATE INDEX ix_expense_splits_user ON tripmate.expense_splits (user_id);

CREATE TABLE tripmate.outbox_events (
    id UUID PRIMARY KEY,
    aggregate_type VARCHAR(40) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(80) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','published','failed','skipped')),
    attempts INT NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_outbox_pending ON tripmate.outbox_events (available_at) WHERE status IN ('pending','failed');
