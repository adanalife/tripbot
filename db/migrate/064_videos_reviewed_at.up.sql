/* reviewed_at is when a person watched the clip and let it air. The pickers
   (!state, !daytime, !find) skip a clip until it is set, so footage that comes
   out of the video pipeline unreviewed stays off the air; a clip reviewed and
   rejected is flagged instead. Season 1 was reviewed before it ever aired.

   No index: every picker already narrows by corpus (and state or date) first,
   so reviewed_at is a filter over rows those reads fetch anyway. */
ALTER TABLE videos ADD COLUMN reviewed_at TIMESTAMP WITH TIME ZONE;
UPDATE videos SET reviewed_at = now() WHERE corpus = 's1';
