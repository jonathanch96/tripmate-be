ALTER TABLE tripmate.users ALTER COLUMN password_hash DROP NOT NULL;
ALTER TABLE tripmate.users ADD COLUMN google_id TEXT;
CREATE UNIQUE INDEX ux_users_google_id ON tripmate.users (google_id) WHERE google_id IS NOT NULL;
