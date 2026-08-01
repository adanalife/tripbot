Log the failing cron job's name as a `job` attribute rather than concatenating it into the slog message, so the message stays a constant that groups in Loki and Sentry.
