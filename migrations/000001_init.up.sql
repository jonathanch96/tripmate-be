CREATE SCHEMA IF NOT EXISTS tripmate;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE tripmate.schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO tripmate.schema_meta (key, value)
VALUES ('bootstrapped_at', now()::text);

