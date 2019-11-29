package moments

import "time"

// Viewings represent a Moment a User saw
type Moment struct {
	Id          int       `db:"id"`
	UserId      int       `db:"user_id"`
	MomentId    int       `db:"moment_id"`
	ViewCount   int       `db:"view_count"`
	Rating      float64   `db:"rating"`
	DateCreated time.Time `db:"date_created"`
}
