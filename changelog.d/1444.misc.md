New `/health/deps` endpoint reports whether Postgres and NATS are usable, so a wedged dependency is visible without dropping the pod out of its Service the way a readiness failure would.
