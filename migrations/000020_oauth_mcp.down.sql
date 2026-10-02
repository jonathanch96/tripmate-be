UPDATE tripmate.expenses SET source = 'manual' WHERE source = 'assistant';
ALTER TABLE tripmate.expenses DROP COLUMN IF EXISTS created_via;
ALTER TABLE tripmate.expenses DROP CONSTRAINT IF EXISTS expenses_source_check;
ALTER TABLE tripmate.expenses ADD CONSTRAINT expenses_source_check CHECK (source IN ('manual','receipt'));
DROP TABLE IF EXISTS tripmate.oauth_tokens;
DROP TABLE IF EXISTS tripmate.oauth_grants;
DROP TABLE IF EXISTS tripmate.oauth_auth_requests;
DROP TABLE IF EXISTS tripmate.oauth_clients;
