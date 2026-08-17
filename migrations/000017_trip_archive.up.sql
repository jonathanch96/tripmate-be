ALTER TABLE tripmate.trips ADD COLUMN is_archived BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE tripmate.trips ADD COLUMN archived_at TIMESTAMPTZ;
