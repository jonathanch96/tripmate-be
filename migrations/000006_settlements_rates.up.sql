CREATE TABLE tripmate.settlements (
    id UUID PRIMARY KEY,
    trip_id UUID NOT NULL REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    from_user_id UUID NOT NULL REFERENCES tripmate.users(id),
    to_user_id UUID NOT NULL REFERENCES tripmate.users(id),
    amount NUMERIC(20,6) NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    method VARCHAR(20) NOT NULL CHECK (method IN ('cash','bank_transfer')),
    bank_name VARCHAR(120),
    bank_account_number VARCHAR(64),
    bank_account_holder VARCHAR(160),
    note TEXT,
    proof_url TEXT,
    status VARCHAR(10) NOT NULL DEFAULT 'approved' CHECK (status IN ('pending','approved','rejected')),
    approved_by_user_id UUID REFERENCES tripmate.users(id),
    approved_at TIMESTAMPTZ,
    rejected_reason TEXT,
    created_by_user_id UUID NOT NULL REFERENCES tripmate.users(id),
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CHECK (from_user_id <> to_user_id)
);
CREATE INDEX ix_settlements_trip ON tripmate.settlements (trip_id, status) WHERE deleted_at IS NULL;

CREATE TABLE tripmate.exchange_rates (
    id UUID PRIMARY KEY,
    trip_id UUID REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    from_currency CHAR(3) NOT NULL,
    to_currency CHAR(3) NOT NULL,
    rate NUMERIC(24,12) NOT NULL CHECK (rate > 0),
    is_final BOOLEAN NOT NULL DEFAULT FALSE,
    source VARCHAR(20) NOT NULL DEFAULT 'manual' CHECK (source IN ('manual','seed','provider')),
    effective_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_currency <> to_currency)
);
CREATE UNIQUE INDEX ux_rates_global ON tripmate.exchange_rates (from_currency, to_currency) WHERE trip_id IS NULL;
CREATE UNIQUE INDEX ux_rates_trip ON tripmate.exchange_rates (trip_id, from_currency, to_currency) WHERE trip_id IS NOT NULL;
