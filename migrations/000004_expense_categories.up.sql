CREATE TABLE tripmate.expense_categories (
    id UUID PRIMARY KEY,
    trip_id UUID REFERENCES tripmate.trips(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ux_expense_categories_global ON tripmate.expense_categories (lower(name)) WHERE trip_id IS NULL;
CREATE UNIQUE INDEX ux_expense_categories_trip ON tripmate.expense_categories (trip_id, lower(name)) WHERE trip_id IS NOT NULL;
CREATE INDEX ix_expense_categories_trip ON tripmate.expense_categories (trip_id);

INSERT INTO tripmate.expense_categories (id, trip_id, name, is_default) VALUES
    (uuid_generate_v4(), NULL, 'Food & Drink', TRUE),
    (uuid_generate_v4(), NULL, 'Transport', TRUE),
    (uuid_generate_v4(), NULL, 'Lodging', TRUE),
    (uuid_generate_v4(), NULL, 'Activities', TRUE),
    (uuid_generate_v4(), NULL, 'Shopping', TRUE),
    (uuid_generate_v4(), NULL, 'Other', TRUE);
