/* Removes chatbot.twitch_gateway (seeded by 021). The prod Twitch Helix cutover
   completed (flag flipped on 2026-07-04, burn-in clean), and the in-process
   fallback + the code-level flag were deleted — the gateway is now the
   unconditional single Helix caller. With nothing reading the flag, the row is
   a dead toggle in the console, so drop it. */
DELETE FROM feature_flags WHERE key = 'chatbot.twitch_gateway';
