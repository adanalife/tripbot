-- Drop the orphan `moments` (006) + `viewings` (007) tables: Dana's original
-- 2020 GPS/geocoding design, never wired up. Verified zero references in Go,
-- templates, scripts, or SQL (pkg/moments was removed in tripbot#542). An old
-- idea we'd redesign from scratch if revived, so the dead schema shouldn't
-- linger. See vault/tripbot/dashcam-cv-plan.md.
DROP TABLE IF EXISTS viewings;
DROP TABLE IF EXISTS moments;
