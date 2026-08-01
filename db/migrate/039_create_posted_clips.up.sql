-- Clips the distribution loop has already shipped, so a scenic-prompt sweep
-- never picks the same stretch of road twice.
--
-- One row per rendered candidate, not per successful post: a dry-run render
-- writes a row with a NULL platform, which is what keeps a week of unarmed
-- observation from re-offering the same clip every night. platform/post_id fill
-- in only once a clip actually lands somewhere.
--
-- Deduping is per video_id rather than per (video_id, ts_sec): two moments from
-- one source file are the same stretch of road, and posting both reads as a
-- repeat even though the timestamps differ.
CREATE TABLE posted_clips (
    id        BIGSERIAL PRIMARY KEY,
    video_id  INTEGER NOT NULL REFERENCES videos(id),
    ts_sec    FLOAT   NOT NULL,          -- centre of the rendered window
    prompt    TEXT    NOT NULL,          -- the scenic prompt that surfaced it
    platform  TEXT,                      -- NULL until posted (dry-run candidate)
    post_id   TEXT,                      -- the platform's id for the post
    posted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The selection query is "has this video been used at all", so index the column
-- it filters on.
CREATE INDEX posted_clips_video ON posted_clips (video_id);

-- A clip posts at most once per platform. Partial, so the NULL-platform dry-run
-- rows don't collide with each other (in SQL every NULL is distinct anyway, but
-- being explicit keeps the intent readable).
CREATE UNIQUE INDEX posted_clips_once_per_platform
    ON posted_clips (video_id, platform)
    WHERE platform IS NOT NULL;
