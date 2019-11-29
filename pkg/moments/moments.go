package moments

import (
	"database/sql"
	"fmt"
	"time"
)

// Moments represent a moment in time in a Video
type Moment struct {
	ID          int           `db:"id"`
	VideoID     int           `db:"video_id"`
	Lat         float64       `db:"lat"`
	Lng         float64       `db:"lng"`
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

// CurrentlyPlaying is the moment that is currently playing
var CurrentlyPlaying Moment

// GetCurrentlyPlaying updates the current moment
func GetCurrentlyPlaying() {
}

//TODO: could be greatly improved
func (m Moment) String() string {
	return fmt.Sprintf("%s->%s", m.ID, m.VideoID)
}

func LoadOrCreate(slug, offset string) (Moment, error) {
	mom := Moment{ID: 0}
	return mom, nil
}
