/* How videos.miles_driven was measured, written beside it by
   cmd/backfill-miles-driven: 'track' is the sum of the steps along the clip's
   own video_coords track, 'chord' the straight line from this clip's fix to
   the next clip's. NULL exactly when miles_driven is. */
ALTER TABLE videos ADD COLUMN miles_driven_source TEXT;
