CREATE TABLE users (
  id             SERIAL PRIMARY KEY,
  //TODO: uniq
  username       VARCHAR(64) NOT NULL,
  /* float32: https://github.com/go-pg/pg/wiki/Model-Definition */
  //TODO: default 0.0
  miles          REAL,
  //TODO: default 0
  num_visits     INTEGER,
  //TODO: default false
  has_donated    BOOLEAN,
  first_seen     TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  last_seen      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
