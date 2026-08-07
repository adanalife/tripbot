-- Where each moment is, in words, so answering "where are we" costs no API call.
--
-- !location, !state and !guess all turn the playhead coordinate into a place
-- name, and every one of those was a live Google Maps reverse-geocode: billed
-- per invocation, dependent on a third party being up, and answering a question
-- whose inputs never change. The corpus is a trip that finished in 2018 — the
-- coordinate for a given moment is fixed forever, so the place name for it is
-- too, and belongs in the row rather than behind a network call.
--
-- The pipeline's geocode pass fills these from Census 2018 cartographic
-- boundary files: state and place polygons of the same vintage as the footage.
-- NULL means the pass hasn't reached this row, which is not the same as "no
-- place here" (city_m distinguishes that) — so consumers fall back to the
-- clip-level videos.state, and to the live geocoder, until it has.
ALTER TABLE video_coords ADD COLUMN state TEXT;
ALTER TABLE video_coords ADD COLUMN city  TEXT;

-- How far this moment is from `city`, in metres. 0 means inside its limits;
-- greater means the nearest one, which is the honest answer for a corpus that
-- is mostly interstate — 57% of moments are outside every incorporated place,
-- with a median of 5 km to the closest.
--
-- Separating the distance from the name is what lets the caller decide when
-- "near" stops being useful without a re-run of the pass: a threshold in code
-- can move, a name already collapsed to NULL cannot. NULL alongside a NULL
-- city means unprocessed; NULL alongside a set city would be a bug.
ALTER TABLE video_coords ADD COLUMN city_m REAL;
