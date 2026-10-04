The Postgres connection URL percent-encodes the database user and password, so a password containing `@`, `/`, `#`, `?` or `%` no longer breaks the DSN.
