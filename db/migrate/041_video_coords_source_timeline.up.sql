-- Key a coordinate to the footage it was read from, not to the cut of it.
--
-- video_coords.ts_sec is an offset into an airing clip, and 122 of those clips
-- are trims out of a longer recording whose start point was *recovered* rather
-- than recorded. Correcting one of those trim points shifts every timestamp
-- inside that clip, which would silently invalidate its whole track: the rows
-- would still be there, still carry a confidence, and describe the wrong
-- places.
--
-- source_ts_sec is the offset into the original recording, which nothing ever
-- re-cuts. It is therefore the stable identity of a coordinate, and the primary
-- key. Re-trimming becomes arithmetic on ts_sec rather than a re-read of the
-- footage.
ALTER TABLE video_coords ADD COLUMN source_ts_sec REAL;

/* Empty until the coords stage first runs, so no backfill is possible or
   needed; for an untrimmed clip the two columns are equal anyway. */
UPDATE video_coords SET source_ts_sec = ts_sec WHERE source_ts_sec IS NULL;
ALTER TABLE video_coords ALTER COLUMN source_ts_sec SET NOT NULL;

ALTER TABLE video_coords DROP CONSTRAINT video_coords_pkey;
ALTER TABLE video_coords ADD PRIMARY KEY (video_id, source_ts_sec);

/* A track covers the whole original, including footage the current trim leaves
   out — which is what lets a future trim widen without re-reading anything. A
   coordinate outside the aired clip has no offset within it, so ts_sec is null
   there, and consumers asking "where was the van at this moment of this clip"
   are answered by the partial index below. */
ALTER TABLE video_coords ALTER COLUMN ts_sec DROP NOT NULL;
CREATE INDEX video_coords_aired_idx ON video_coords (video_id, ts_sec)
    WHERE ts_sec IS NOT NULL;
