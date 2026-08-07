-- Where the van was at each moment of a clip, rather than once per clip.
--
-- videos.lat/lng carries one coordinate for a whole three-minute clip, which is
-- three to five kilometres of driving. That is fine for asking which state a
-- clip was filmed in, and wrong for anything that points at a moment: guessr
-- shows a three-second cut from somewhere inside the clip and answers it with
-- the clip's single coordinate, so a player who identifies the exact street is
-- told they missed by a mile.
--
-- The dashcam prints its coordinate into every frame, so the footage already
-- knows; video-pipeline's coords stage reads it back out at the same 2 s grid
-- frame_embeddings samples, and writes it here.
--
-- Rows are dense: every sampled moment gets one, so a consumer can look up a
-- timestamp without checking whether that particular read happened to succeed.
-- `source` says whether a row was read off the frame or inferred from the reads
-- around it, so a consumer that cares can insist on the former.
CREATE TABLE video_coords (
    video_id INTEGER NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    ts_sec   REAL NOT NULL,
    lat      DOUBLE PRECISION NOT NULL,
    lng      DOUBLE PRECISION NOT NULL,
    -- ocr          - read off this frame's overlay and believed
    -- interpolated - inferred from the surviving reads either side
    source   TEXT NOT NULL,
    PRIMARY KEY (video_id, ts_sec)
);

/* How much the clip's track is worth believing, 0..1. A clip whose overlay
   mostly failed to read, or whose surviving reads imply a path no vehicle could
   have driven, still gets rows -- interpolation always produces something -- and
   this is the only thing that says so. Consumers picking a moment to show
   someone should require a high value; NULL means the coords stage has not
   looked at this clip yet. */
ALTER TABLE videos ADD COLUMN coord_confidence REAL;

/* How far off this clip's least-certain moment probably is, in metres, taken
   from the longest stretch of frames whose overlay could not be read. Answers
   "is this clip precise enough for what I am about to do with it" without the
   consumer needing to know how the track was built: showing someone a moment on
   a map wants a few tens of metres, deciding which state it was in does not
   care. NULL until the coords stage has processed the clip. */
ALTER TABLE videos ADD COLUMN coord_accuracy_m REAL;

/* Tracks are per-moment, so a clip that has one no longer derives its
   representative coordinate from a single frame's read. */
COMMENT ON COLUMN videos.coord_source IS
    'ocr | interpolated | rejected | missing | track (median of a video_coords track)';
