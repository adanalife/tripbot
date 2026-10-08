DROP INDEX frame_embeddings_dedupe;
/* Safe to key on the clip clock again: a clip's trim offset is constant, so two
   distinct source moments never collapse onto one ts_sec. */
CREATE UNIQUE INDEX frame_embeddings_dedupe
    ON frame_embeddings (video_id, ts_sec, model);
ALTER TABLE frame_embeddings DROP COLUMN source_ts_sec;
