-- Recreate moments (006) + viewings (007) verbatim from their original bodies
-- for reversibility.
CREATE TABLE moments (
  id             SERIAL PRIMARY KEY,
  video_id       INTEGER NOT NULL,
  next_moment    INTEGER,
  prev_moment    INTEGER,
  lat            FLOAT DEFAULT 0,
  lng            FLOAT DEFAULT 0,
  address        VARCHAR,
  locality       VARCHAR,
  region         VARCHAR,
  postcode       VARCHAR,
  country        VARCHAR,
  flagged        BOOLEAN DEFAULT FALSE,
  time_offset    VARCHAR(10),
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE viewings (
  id             SERIAL PRIMARY KEY,
  user_id        INTEGER NOT NULL,
  moment_id      INTEGER NOT NULL,
  view_count     INTEGER NOT NULL DEFAULT 0,
  rating         FLOAT DEFAULT 'NaN',
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
