/* Removes chatbot.find (seeded by 032, extended by 034) and chatbot.weather
   (seeded by 017, backfilled to youtube by 019). Both commands ran behind a
   dormant-ship gate while their dependencies were being proven — the
   video-pipeline embed responder for !find, the Open-Meteo archive lookup for
   !weather. Both are stable, so the gates came out of the Go code; with nothing
   reading these keys the rows are dead toggles in the console, and a dead
   toggle that looks live is worse than no toggle. */
DELETE FROM feature_flags WHERE key IN ('chatbot.find', 'chatbot.weather');
