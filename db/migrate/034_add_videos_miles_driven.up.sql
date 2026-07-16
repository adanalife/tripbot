/* Real road distance (in miles) the van covered during this clip: the
   great-circle distance from this clip's GPS fix to the next clip's fix in
   film order, written by cmd/backfill-miles-driven. NULL = unknowable (no
   usable fix on one side of the pair, or a recording gap too large to
   attribute the distance to this clip). Feeds the real-miles rollup, and
   SUM(miles_driven) is the whole-trip odometer. */
ALTER TABLE videos ADD COLUMN miles_driven REAL;
