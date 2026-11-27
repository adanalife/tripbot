/* The backdrop-free warp is the default on every platform: the wordmark and
   streaks over the live video, outlined so they read over bright footage too.
   The opaque cover (the row off) stays one console flip away for a platform
   whose playhead-jump gap shows. */
UPDATE feature_flags SET enabled = TRUE WHERE key = 'chatbot.timewarp_no_background';
