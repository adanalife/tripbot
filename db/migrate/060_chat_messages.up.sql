/* The chat log: one row per line seen in chat — a viewer's message or the
   bot's own send — stamped with what was airing, so "what did they see" is a
   read rather than a reconstruction against video_plays. Append-only like
   events. video_id / video_ts_sec are NULL when the writing instance had no
   playhead. The console pages this newest-first by id, for all platforms or
   one, which (platform, id) serves. */
CREATE TABLE chat_messages (
  id           BIGSERIAL PRIMARY KEY,
  platform     TEXT NOT NULL DEFAULT '',
  username     TEXT NOT NULL,
  user_id      TEXT NOT NULL DEFAULT '',
  message_id   TEXT NOT NULL DEFAULT '',
  text         TEXT NOT NULL,
  video_id     INTEGER,
  video_ts_sec REAL,
  emitted_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX chat_messages_platform_id_idx ON chat_messages (platform, id DESC);
