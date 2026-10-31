-- The album bed's selection and play order, one row per streaming platform, so
-- a restart resumes the choice an operator made. OBS can only report the file
-- it is playing, which names one album, never the group or whole-share
-- selection it was drawn from.
CREATE TABLE background_audio_prefs (
    platform   TEXT PRIMARY KEY,
    album      TEXT NOT NULL,
    shuffle    BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
