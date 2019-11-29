CREATE TABLE viewings (
  id             SERIAL PRIMARY KEY,
  user_id        INTEGER NOT NULL,
  moment_id      INTEGER NOT NULL,
  view_count     INTEGER NOT NULL DEFAULT 0,
  rating         FLOAT DEFAULT 'NaN',
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
