-- The chatter's platform-native user id, alongside the display name.
--
-- A username is the only handle the users table has ever had, and on Twitch
-- (and YouTube, and every platform behind the gateway) it is a mutable display
-- login: a viewer who renames leaves their row stranded under the old name and
-- gets a fresh one under the new, splitting their miles and their history. The
-- platform's own id is the stable half of the identity, and the inbound chat
-- envelope has carried it since the gateway cutover — it just had nowhere to go.
ALTER TABLE users
  ADD COLUMN platform_user_id TEXT;

-- Nullable and unbackfilled on purpose: no historical row has an id to fill in
-- from, and every active chatter's row gets stamped on their next message. So
-- the column fills in organically, and an unset value means "not seen since
-- this shipped" rather than "no such user".
--
-- Partial unique index: two rows on one platform must never claim the same id
-- (that would be the rename bug in reverse, two names fighting over one
-- identity), while the many rows that are still unset can't collide under it.
-- It doubles as the lookup index for resolving a profile by id.
--
-- The predicate has to exclude the empty string as well as NULL. Unset arrives
-- as either depending on the writer — GORM sends the Go zero value '' for a
-- string field, a hand-written INSERT omitting the column gets NULL — and an
-- index guarding only NULL lets the first two GORM-written rows collide on ''.
CREATE UNIQUE INDEX users_platform_user_id_key
  ON users (platform, platform_user_id)
  WHERE platform_user_id IS NOT NULL AND platform_user_id <> '';
