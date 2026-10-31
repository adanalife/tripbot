/* Removes the chatbot.youtube_gateway flag seeded by 023. YouTube outbound
   chat-send now routes through the platform-gateway (gateway-youtube)
   unconditionally whenever YOUTUBE_API_URL is wired — there is no runtime
   toggle, so the flag row is dead weight (and a console toggle that does
   nothing). Unlike the Twitch cutover, YouTube has no live-prod stakes that
   warrant a flag; a revert is a git revert + redeploy.

   Safe across a rollout: the pre-deletion code reads the flag as off when the
   row is gone (in-process send), and the post-deploy code ignores the flag
   entirely. */
DELETE FROM feature_flags WHERE key = 'chatbot.youtube_gateway';
