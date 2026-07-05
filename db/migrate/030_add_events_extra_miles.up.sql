/* Records the bonus portion of a session that the events login/logout pairing
   can't reconstruct: community sub-grants received (the +1-to-everyone on a
   sub) plus the 5% subscriber bonus. Populated on logout events only, and left
   NULL when zero (most sessions) — SUM treats NULL and 0 identically, so a
   future rollup can add SUM(extra_miles_earned) to the events-derived base. */
ALTER TABLE events ADD COLUMN extra_miles_earned DOUBLE PRECISION;
