-- The clip's own place name, so the fallback path costs no API call either.
--
-- Migration 045 gave every *moment* a place, which answers !location, !state and
-- !guess from the row for the 4,212 of 4,406 clips whose per-moment track is
-- trustworthy enough to read. The other 194 fall back to the clip's single
-- representative coordinate — and `videos` carries a `state` for that but no
-- city, so a live Google Maps reverse-geocode was the only way to name one.
--
-- That fallback is also what the ambient location feed reads, so it fires on a
-- 24/7 stream rather than only when someone types a command.
--
-- The pipeline's geocode pass fills these from the same Census boundary files
-- of the same vintage as the footage, naming videos.lat/lng exactly as it names
-- a moment. NULL means the pass hasn't reached the row.
ALTER TABLE videos ADD COLUMN city TEXT;

-- Distance from `city` in metres, 0 meaning inside its limits — the same
-- meaning as video_coords.city_m, so both render through one code path.
-- Without it a clip 100 km from the nearest town would read as though it were
-- in it, which is the failure separating name from distance exists to prevent.
ALTER TABLE videos ADD COLUMN city_m REAL;
