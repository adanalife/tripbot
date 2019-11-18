## Connect to the DB

`psql <postgres://url>`

## To create a new migration

`migrate create -ext sql -seq -digits 3 -dir db/migrate <migration_name>`

## To run a migration

`migrate -database <postgres://url> -source file://./db/migrate up <migration_number>`


## To take a backup

`pg_dump <postgres://url> > db_dump.<date>.sql`


## Restore from backup
```
psql postgres://e <postgres://url> < db_dump.sql
```
