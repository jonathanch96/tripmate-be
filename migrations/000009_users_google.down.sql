DROP INDEX IF EXISTS tripmate.ux_users_google_id;
ALTER TABLE tripmate.users DROP COLUMN IF EXISTS google_id;
ALTER TABLE tripmate.users ALTER COLUMN password_hash SET NOT NULL;
