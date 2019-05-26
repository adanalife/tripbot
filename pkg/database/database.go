package database

import (
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	// this is how we will share the DB connection
	DBCon *sqlx.DB
)

type Event struct {
	Username    string    `db:"username"`
	Event       string    `db:"event"`
	DateCreated time.Time `db:"date_created"`
}
