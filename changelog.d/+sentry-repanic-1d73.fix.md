A panicking HTTP handler in tripbot or onscreens-server answers 500 and counts toward the HTTP panic metric, instead of being swallowed by the Sentry middleware and returning an empty 200.
