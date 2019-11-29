package moments

import "time"

// Viewings represent a Moment a User saw
type Viewing struct {
	ID          int       `db:"id"`
	UserID      int       `db:"user_id"`
	MomentID    int       `db:"moment_id"`
	ViewCount   int       `db:"view_count"`
	Rating      float64   `db:"rating"`
	DateCreated time.Time `db:"date_created"`
}
