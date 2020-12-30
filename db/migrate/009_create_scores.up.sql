CREATE TABLE scores (
  id            SERIAL PRIMARY KEY,
  user_id       INTEGER REFERENCES users(id),
  scoreboard_id INTEGER REFERENCES scoreboards(id),
  value         REAL DEFAULT 0.0, /* float32: https://github.com/go-pg/pg/wiki/Model-Definition */
  date_created  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
