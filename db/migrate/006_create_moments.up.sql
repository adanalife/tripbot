CREATE TABLE moments (
  id             SERIAL PRIMARY KEY,
  video_id       INTEGER NOT NULL,
  next_moment    INTEGER,
  prev_moment    INTEGER,
  lat            FLOAT DEFAULT 0,
  lng            FLOAT DEFAULT 0,
  rating         FLOAT DEFAULT 0,
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
