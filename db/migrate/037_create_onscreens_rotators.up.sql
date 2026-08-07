-- Console-edited corner-rotator copy, one row per streaming platform.
--
-- The whole config is one JSONB document rather than a row per message: the
-- console edits a whole list at a time (add/remove/reorder), and a single
-- document makes that a plain overwrite instead of a diff, with message order
-- preserved by the array itself. A platform with no row here renders the copy
-- compiled into onscreens-server.
CREATE TABLE onscreens_rotators (
    platform   TEXT PRIMARY KEY,
    config     JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
