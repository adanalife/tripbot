/* GPS coordinate provenance. videos.lat/lng holds a single per-clip fix from
   the (long-removed) dashcam-overlay OCR pass, with no record of how
   trustworthy each point was. cmd/backfill-coords rejects digit-flip outliers
   and interpolates gaps from neighbouring clips; coord_source records which
   points are original OCR fixes vs. synthesized vs. discarded, so the map can
   style them and a future re-OCR pass won't mistake interpolated points for
   real fixes. Values:
     ocr          - original dashcam-overlay OCR fix (default)
     interpolated - synthesized from neighbouring clips by backfill-coords
     rejected     - OCR fix discarded as an impossible-speed outlier (coords cleared, flagged)
     missing      - no GPS fix, none recoverable */
ALTER TABLE videos ADD COLUMN coord_source TEXT NOT NULL DEFAULT 'ocr';

/* Rows tripbot inserted at runtime always have lat/lng = 0 and flagged = true
   (OCR was removed years ago; see pkg/video/db.go save()). Mark those missing
   rather than mislabelling them as OCR fixes. */
UPDATE videos SET coord_source = 'missing' WHERE flagged OR (lat = 0 AND lng = 0);
