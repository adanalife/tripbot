/* What the pipeline's auto-trim proposed for each season-2 piece, and the
   reviewer's ruling over it, so the console can show a source's cuts. One
   row per piece, aired or not: a dropped piece has no videos row, so this
   keys on the piece's slug (<stem>_s<offset>), which is its clip's slug
   when it airs.

   proposal is auto-trim's row as video-cuts.yml carries it (duration,
   verdict, mean_similarity, keep and cuts spans in the piece's own
   timeline); ruling is the review page's row for the piece, NULL until
   someone rules. The pipeline's per-source finish step rewrites a
   source's rows; nothing reads them to decide what airs. */
CREATE TABLE video_cuts (
  slug       TEXT PRIMARY KEY,
  proposal   JSONB NOT NULL,
  ruling     JSONB,
  updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
