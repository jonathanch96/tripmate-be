CREATE TABLE tripmate.receipts (
    id             UUID PRIMARY KEY,
    trip_id        UUID NOT NULL REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    expense_id     UUID REFERENCES tripmate.expenses(id) ON DELETE SET NULL,
    uploaded_by    UUID NOT NULL REFERENCES tripmate.users(id),
    storage_key    TEXT NOT NULL,
    content_type   VARCHAR(40) NOT NULL,
    size_bytes     BIGINT NOT NULL,
    merchant       VARCHAR(200),
    receipt_date   DATE,
    currency       CHAR(3),
    subtotal       NUMERIC(20,6),
    tax            NUMERIC(20,6),
    service_charge NUMERIC(20,6),
    total          NUMERIC(20,6),
    provider       VARCHAR(40),
    provider_model VARCHAR(80),
    raw_response   JSONB,
    status         VARCHAR(20) NOT NULL DEFAULT 'uploaded'
        CHECK (status IN ('uploaded','extracting','extracted','failed','converted')),
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX ix_receipts_trip ON tripmate.receipts (trip_id) WHERE deleted_at IS NULL;
CREATE INDEX ix_receipts_expense ON tripmate.receipts (expense_id);

CREATE TABLE tripmate.receipt_items (
    id          UUID PRIMARY KEY,
    receipt_id  UUID NOT NULL REFERENCES tripmate.receipts(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    quantity    NUMERIC(12,3) NOT NULL DEFAULT 1,
    unit_price  NUMERIC(20,6) NOT NULL DEFAULT 0,
    total_price NUMERIC(20,6) NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_receipt_items_receipt ON tripmate.receipt_items (receipt_id, sort_order);

CREATE TABLE tripmate.receipt_item_assignments (
    id              UUID PRIMARY KEY,
    receipt_item_id UUID NOT NULL REFERENCES tripmate.receipt_items(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES tripmate.users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_receipt_item_assignments
    ON tripmate.receipt_item_assignments (receipt_item_id, user_id);
