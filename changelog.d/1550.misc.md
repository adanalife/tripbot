The oauth_tokens read carries its caller's context, so it emits a trace span and honours cancellation like every other query.
