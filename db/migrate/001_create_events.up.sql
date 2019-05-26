CREATE TABLE events (
  id             SERIAL PRIMARY KEY,
  username       VARCHAR(64) NOT NULL,
  event          VARCHAR(64) NOT NULL,
  date_created   TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
