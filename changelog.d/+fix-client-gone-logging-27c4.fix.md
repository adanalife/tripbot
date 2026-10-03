A read API query cancelled because its caller hung up (the console gives up after 2s) logs a warning rather than an error, so it no longer opens Sentry events.
