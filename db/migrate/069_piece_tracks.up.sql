/* The GPS track of a season-2 piece that has no clip — auto-trim dropped it
   whole, so it never got a videos row for video_coords to key on — read off
   the piece itself by the pipeline's track step, so the console can still
   draw where it went. One row per piece, keyed like video_cuts by the piece's
   slug. points is [[sec, lat, lng], ...] on the piece's own clock;
   confidence is the coords pass's score, gated on the way out the same as
   videos.coord_confidence. */
CREATE TABLE piece_tracks (
  slug       TEXT PRIMARY KEY,
  confidence REAL NOT NULL,
  points     JSONB NOT NULL,
  updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
