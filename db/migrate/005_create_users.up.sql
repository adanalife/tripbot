CREATE TABLE users (
  id             SERIAL PRIMARY KEY,
  username       VARCHAR(64) UNIQUE NOT NULL,
  miles          REAL DEFAULT 0.0, /* float32: https://github.com/go-pg/pg/wiki/Model-Definition */
  num_visits     INTEGER DEFAULT 0,
  has_donated    BOOLEAN DEFAULT FALSE,
  first_seen     TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  last_seen      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
