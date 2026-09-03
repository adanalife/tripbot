/* videos.state carries no index, so every by-state lookup is a sequential scan:
   FindRandomByState's COUNT(*) and its offset skip, !find's state facet, and
   !jump <state>'s filter all read the whole table. Cheap at today's corpus, and
   the cost grows with every clip ingested.

   CONCURRENTLY so building it doesn't lock writes on the live videos table.
   golang-migrate runs migrations without a transaction wrapper, so CONCURRENTLY
   is allowed — but this file must stay a SINGLE statement (a multi-statement
   file would be sent in one implicit transaction). */
CREATE INDEX CONCURRENTLY IF NOT EXISTS videos_state_idx ON videos (state);
