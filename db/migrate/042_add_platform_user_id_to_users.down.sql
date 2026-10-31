DROP INDEX IF EXISTS users_platform_user_id_key;

ALTER TABLE users
  DROP COLUMN platform_user_id;
