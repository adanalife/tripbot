CREATE TABLE oauth_tokens (
  id                  SERIAL PRIMARY KEY,
  provider            TEXT NOT NULL DEFAULT 'twitch',
  username            TEXT NOT NULL,
  twitch_user_id      TEXT,
  access_token        TEXT NOT NULL,
  refresh_token       TEXT NOT NULL,
  expires_at          TIMESTAMP WITH TIME ZONE NOT NULL,
  scopes              TEXT NOT NULL,
  refresh_fail_count  INTEGER NOT NULL DEFAULT 0,
  last_refresh_at     TIMESTAMP WITH TIME ZONE,
  date_created        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  date_updated        TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (provider, username)
);
