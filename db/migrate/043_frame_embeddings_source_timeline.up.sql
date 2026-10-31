-- Key an embedding to the footage it was read from, not to the cut of it.
--
-- frame_embeddings.ts_sec is an offset into an airing clip, and 122 of those
-- clips are trims out of a longer recording whose start point was *recovered*
-- rather than recorded. Correcting one of those trim points shifts every
-- timestamp inside that clip, so its vectors would silently describe different
-- frames than their ts_sec claims. Nothing downstream would notice: guessr cuts
-- its round clip at -ss ts_sec, so the picture a player sees stops matching the
-- coordinate they are graded against, and !find deep-links a viewer to a moment
-- that has moved.
--
-- source_ts_sec is the offset into the original recording, which nothing ever
-- re-cuts, and is therefore the vector's stable identity. Re-trimming becomes
-- arithmetic on ts_sec rather than another pass over the footage — which for
-- embeddings is a multi-day CPU-bound job, so paying for it twice is the thing
-- this avoids. Mirrors 041 for video_coords.
ALTER TABLE frame_embeddings ADD COLUMN source_ts_sec FLOAT;

/* A clip's trim offset is the constant difference between the two clocks its
   coords track already carries, so the backfill needs neither the trim manifest
   nor the footage. The difference is exactly constant within every clip that
   has a track, which is 4392 of 4406. */
UPDATE frame_embeddings fe
SET source_ts_sec = fe.ts_sec + o.offset_sec
FROM (
    SELECT video_id, min(source_ts_sec - ts_sec) AS offset_sec
    FROM video_coords
    WHERE ts_sec IS NOT NULL
    GROUP BY video_id
) o
WHERE o.video_id = fe.video_id;

/* Clips with no coords track yet. The two clocks agree unless the clip is a
   trim, and exactly one trimmed clip in the corpus has no track:
   2018_0611_164057_004_b_opt, whose real offset is 171.75. Its existing rows
   carry the identity placeholder until a coords run reaches it. The error
   cannot spread, because the embed stage refuses to write new rows for a
   trim-suffixed clip whose offset it cannot derive. */
UPDATE frame_embeddings SET source_ts_sec = ts_sec WHERE source_ts_sec IS NULL;
ALTER TABLE frame_embeddings ALTER COLUMN source_ts_sec SET NOT NULL;

/* Dedupe on the stable clock: two rows describing the same moment of the same
   recording are the same vector however that recording is currently cut. */
DROP INDEX frame_embeddings_dedupe;
CREATE UNIQUE INDEX frame_embeddings_dedupe
    ON frame_embeddings (video_id, source_ts_sec, model);
