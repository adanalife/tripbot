
## How to make a DB dumb

```
pg_dump postgres://USER:PASS@dmerrick-void.local/tripbot_prod?sslmode=disable > db_dump.sql
```

## Migrate the db

```
migrate -database postgres://USER:PASS@dmerrick-void.local/tripbot_dev?sslmode=disable -source file://./db/migrate up
```


## Connect to the db

```
psql postgres://USER:PASS@dmerrick-void.local/tripbot_prod?sslmode=disable
```

