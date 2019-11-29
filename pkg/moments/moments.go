package moments

import (
	"database/sql"
	"time"
)

// Moments represent a moment in time in a Video
type Moment struct {
	Id          int           `db:"id"`
	VideoId     int           `db:"video_id"`
	Lat         float64       `db:"lat"`
	Lng         float64       `db:"lng"`
	Rating      float64       `db:"rating"`
	Address     string        `db:"address"`
	Locality    string        `db:"locality"`
	Region      string        `db:"region"`
	Postcode    string        `db:"postcode"`
	Country     string        `db:"country"`
	Flagged     bool          `db:"flagged"`
	TimeOffset  string        `db:"time_offset"`
	NextMoment  sql.NullInt64 `db:"next_moment"`
	PrevMoment  sql.NullInt64 `db:"prev_moment"`
	DateCreated time.Time     `db:"date_created"`
}
