## Initialize the db

`createdb $DATABASE_DB`

## Connect to the DB

`psql postgres://$DATABASE_USER:$DATABASE_PASS@$DATABASE_HOST/$DATABASE_DB`

## To run a migration

`migrate -database <postgres://url> -source file://./db/migrate up <migration_number>`

## To create a new migration

`migrate create -ext sql -seq -digits 3 -dir db/migrate <migration_name>`

## To take a backup

`pg_dump <postgres://url> > db_dump.<date>.sql`


## Restore from backup
```
psql <postgres://url> < db_dump.sql
```
