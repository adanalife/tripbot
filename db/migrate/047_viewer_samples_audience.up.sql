-- viewer_samples.count has always held the *chatter* total — how many people
-- have spoken in chat. That is a small, self-selecting slice of the audience,
-- and it biases toward whatever provokes typing rather than whatever holds
-- attention, so it answers "which footage performs best" badly.
--
-- viewers is the platform's concurrent-viewer number, read from the gateway's
-- /v1/viewers. NULL means nobody reported one for that tick: a platform with
-- no such endpoint, a gateway too old to serve it, or a failed call. It is a
-- new column rather than a redefinition of count because the chatter series
-- collected since 2026-07-06 stays readable only if its meaning doesn't move
-- underneath it.
ALTER TABLE viewer_samples ADD COLUMN viewers INTEGER;

-- Whether anything was broadcasting at sample time. An offline channel and a
-- live one with an empty room both report zero viewers and mean opposite
-- things; without this, a rollup averaging the samples reads every offline
-- hour as footage that shed its audience. NULL wherever viewers is NULL —
-- the same "nobody reported" case.
ALTER TABLE viewer_samples ADD COLUMN live BOOLEAN;
