/* Three readers walk videos in film order and none of them is indexed:
   FindNextDaytime (`corpus = ? AND date_filmed > ? ORDER BY date_filmed`) and
   the backfill-coords / backfill-miles-driven walks (`ORDER BY corpus,
   date_filmed, id`). Leading with corpus serves all three, since each stays
   inside one corpus, and a date-only range still has date_filmed second.

   CONCURRENTLY so building it doesn't lock writes on the live videos table.
   golang-migrate runs migrations without a transaction wrapper, so CONCURRENTLY
   is allowed — but this file must stay a SINGLE statement (a multi-statement
   file would be sent in one implicit transaction). */
CREATE INDEX CONCURRENTLY IF NOT EXISTS videos_corpus_date_filmed_idx
  ON videos (corpus, date_filmed);
