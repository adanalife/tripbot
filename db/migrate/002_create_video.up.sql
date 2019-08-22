CREATE TABLE videos (
  id             SERIAL PRIMARY KEY,
  slug           VARCHAR(128) NOT NULL,
  lat            FLOAT DEFAULT 0,
  lng            FLOAT DEFAULT 0,
  next_vid       INTEGER,
  prev_vid       INTEGER,
  flagged        BOOLEAN DEFAULT false,
  date_filmed    TIMESTAMP WITH TIME ZONE,
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
