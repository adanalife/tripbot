/* Irreversible by design: the backfill can't tell a row it created from one
   that already existed, and deleting a flag row switches its feature off with
   no way back through the console. Roll a flag back by toggling it, not by
   reverting this. */
SELECT 1;
