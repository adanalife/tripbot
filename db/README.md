On a Mac, it's probably easiest to use Postgres.app.

On Linux:

```bash
sudo apt install postgres
sudo -u postgres psql
> CREATE DATABASE "tripbot_dev";
> CREATE USER tripbot_dev WITH PASSWORD 'some password';
> GRANT ALL PRIVILEGES ON DATABASE "tripbot_dev" to tripbot_dev;
```

## Connect to the DB

`psql postgres://$DATABASE_USER:$DATABASE_PASS@$DATABASE_HOST/$DATABASE_DB`

Development example:
`psql postgres://tripbot_docker:hunter2@$(./bin/devenv port db 5432)/tripbot_docker`

You might need to add `?sslmode=disable` for local development servers.

## To run a migration

Migrations are run using [golang-migrate](https://github.com/golang-migrate/migrate). Install with `brew install golang-migrate` on macOS.

Apply all pending migrations:

`migrate -database <postgres://url> -source file://./db/migrate up`

Apply a specific number of pending migrations:

`migrate -database <postgres://url> -source file://./db/migrate up <migration_number>`

Roll back the most recent migration:

`migrate -database <postgres://url> -source file://./db/migrate down 1`

Show the current schema version:

`migrate -database <postgres://url> -source file://./db/migrate version`

## To create a new migration

`migrate create -ext sql -seq -digits 3 -dir db/migrate <migration_name>`

## Migrations of note

- `010_create_oauth_tokens` — stores Twitch OAuth refresh + access tokens for the bot account. Populate via `task tripbot:auth:bootstrap` (one-time, local). The bot reads the row at boot and refreshes hourly.

## To take a backup

`pg_dump <postgres://url> > db_dump.$(date "+%Y%m%d").sql`


## Restore from backup

`psql <postgres://url> < db_dump.sql`

## Import from seed

### Via devenv

```bash
# the docker-compose file has a seed container set up
devenv up seed
```


### Via psql

```sql
\copy videos FROM 'db/seed/videos.csv' DELIMITER ',' CSV HEADER;
```

#### Other

Get leaderboard winners

```sql
select users.username, scores.value from scoreboards, scores, users where scoreboards.name = 'guess_state_2021_01' and scores.user_id = users.id and scores.scoreboard_id = scoreboards.id ORDER BY scores.value DESC LIMIT 10;
```
