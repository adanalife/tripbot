On a Mac, it's probably easiest to use Postgres.app.

On Linux:

```
sudo apt install postgres
sudo -u postgres psql
> CREATE DATABASE "tripbot_dev";
> CREATE USER tripbot_dev WITH PASSWORD 'some password';
> GRANT ALL PRIVILEGES ON DATABASE "tripbot_dev" to tripbot_dev;
```

## Connect to the DB

`psql postgres://$DATABASE_USER:$DATABASE_PASS@$DATABASE_HOST/$DATABASE_DB`

## To run a migration

Migrations are run using [go-migrate](https://github.com/golang-migrate/migrate).

`migrate -database <postgres://url> -source file://./db/migrate up <migration_number>`

## To create a new migration

`migrate create -ext sql -seq -digits 3 -dir db/migrate <migration_name>`

## To take a backup

`pg_dump <postgres://url> > db_dump.<date>.sql`


## Restore from backup
```
psql <postgres://url> < db_dump.sql
```
