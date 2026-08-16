/* Real road miles the van drove while this user watched: the sum of
   videos.miles_driven over the video_plays rows that fell inside the user's
   paired sessions. Purely derived (rebuildable like every user_rollups
   column). Coverage starts at the video_plays epoch (migration 033) —
   earlier sessions had no play timeline and contribute nothing. */
ALTER TABLE user_rollups ADD COLUMN real_miles REAL NOT NULL DEFAULT 0.0;
