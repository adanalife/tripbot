/* Per-user SUM(extra_miles_earned) from logout and correction events — the
   bonus/adjustment portion the pairing-derived events_miles can't see
   (community sub-grants + the 5% subscriber bonus + manual corrections). Kept
   as a separate column so events_miles stays the pure pairing base (its delta
   from users.miles is the drift alarm); reconstructed display miles ≈
   events_miles + extra_miles. */
ALTER TABLE user_rollups ADD COLUMN extra_miles REAL NOT NULL DEFAULT 0.0;
