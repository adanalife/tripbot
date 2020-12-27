CREATE TABLE scores (
  id            SERIAL PRIMARY KEY,
  user_id       INTEGER NOT NULL,
  scoreboard_id INTEGER NOT NULL,
  score         REAL DEFAULT 0.0, /* float32: https://github.com/go-pg/pg/wiki/Model-Definition */
  date_created  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
