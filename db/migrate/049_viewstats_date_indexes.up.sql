/* The footage insights query aggregates viewer_samples and video_plays over a
   recent window across all platforms, and the 033 indexes both lead with
   platform — unusable for a date-only range, leaving a full-table scan that
   grows with history. Plain date indexes bound the scan to the window.

   Plain CREATE INDEX (not CONCURRENTLY) is fine here: these tables accrue one
   sample per ~61s per platform and one play per clip switch, so the build is
   momentary and the writers it briefly blocks are best-effort ticks. */
CREATE INDEX viewer_samples_sampled_at ON viewer_samples (sampled_at);
CREATE INDEX video_plays_started_at ON video_plays (started_at);
