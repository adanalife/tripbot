package video

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
)

// minCoordConfidence is how believable a clip's per-moment track has to be
// before a moment inside it is worth answering with. Interpolation always
// produces rows, so a clip whose overlay mostly failed to read still has a
// full track — videos.coord_confidence is the only thing that says so, and
// below this the clip's single fix is the more honest answer. 0.8 keeps 4,023
// of the corpus's 4,354 clips (measured 2026-08-05).
const minCoordConfidence = 0.8

// coordWindow is how far either side of a moment, in seconds, a coordinate may
// be read from. The track is sampled on a 2 s grid, so a row this close is the
// same stretch of road; past it the track has a gap and the clip's single fix
// beats extrapolating across it.
const coordWindow = 10.0

// ocrWindow is how far from the moment asked about, in seconds, a row read off
// the frame may be and still beat a nearer interpolated one. Inside it both
// rows describe the same few metres of road and the measured one is worth
// more; outside it, proximity wins.
const ocrWindow = 1.5

// nearPlaceLimit is how far from a named place a moment can be and still be
// worth naming it. Past this "near X" stops meaning anything and the state is
// the more honest answer. The corpus is mostly interstate — 57% of moments sit
// outside every incorporated place, at a median 5 km from the nearest — so this
// threshold decides the wording for a large share of what !location says: at
// 10 km it names a place for about 81% of moments overall.
const nearPlaceLimit = 10000

// Moment is where the van was at one point inside a clip: the coordinate, plus
// the place the pipeline resolved for it offline. The place fields are empty
// until the geocode pass has reached the row.
type Moment struct {
	Lat, Lng float64

	State string
	City  string
	// CityM is how far the coordinate is from City, in metres; 0 means inside
	// its limits. Meaningless when City is empty.
	CityM float64
}

// Place renders the moment the way chat should hear it: inside a place, near
// one, or — when the nearest is too far to be a landmark — the state alone.
// Empty when the geocode pass hasn't reached this moment, which is the caller's
// signal to fall back to the live geocoder.
//
// The wording matches what pkg/geo's City returned, so replacing that lookup
// doesn't change how !location reads.
func (m Moment) Place() string {
	switch {
	case m.City != "" && m.CityM == 0:
		return fmt.Sprintf("%s, %s", m.City, m.State)
	case m.City != "" && m.CityM <= nearPlaceLimit:
		return fmt.Sprintf("near %s, %s", m.City, m.State)
	case m.State != "":
		return fmt.Sprintf("Somewhere in %s", m.State)
	}
	return ""
}

// The two ORDER BY booleans do the preference in one pass: Postgres sorts
// false before true, so rows within ocrWindow come first, then within those
// the ones read off the frame, then nearest wins.
const coordAtQuery = `
SELECT lat, lng, state, city, city_m FROM video_coords
WHERE video_id = @vid AND ts_sec IS NOT NULL AND abs(ts_sec - @ts) <= @window
ORDER BY abs(ts_sec - @ts) > @ocr, source <> 'ocr', abs(ts_sec - @ts)
LIMIT 1`

// CoordAt returns where the van was `at` into vid, from the per-moment track
// in video_coords. ok is false when the clip has no track worth believing, or
// none covering that moment, and the caller falls back to vid.Location() — the
// single clip-level fix, which sits a median 598 m from where the van actually
// was at a given moment inside the clip (p90 1.9 km, max 4.4 km).
func CoordAt(ctx context.Context, vid Video, at time.Duration) (Moment, bool) {
	if vid.ID == 0 || vid.CoordConfidence == nil || *vid.CoordConfidence < minCoordConfidence {
		return Moment{}, false
	}
	// A negative offset means the caller's clock disagrees with playout's
	// about which clip is on screen; the top of this one is the closest thing
	// to an answer.
	tsSec := max(at.Seconds(), 0)

	// The place columns are null until the geocode pass fills them, so they
	// scan through sql.Null* rather than straight into Moment.
	var row struct {
		Lat, Lng float64
		State    sql.NullString
		City     sql.NullString
		CityM    sql.NullFloat64
	}
	res := database.GormDB().WithContext(ctx).Raw(coordAtQuery,
		sql.Named("vid", vid.ID),
		sql.Named("ts", tsSec),
		sql.Named("window", coordWindow),
		sql.Named("ocr", ocrWindow),
	).Scan(&row)
	if res.Error != nil {
		slog.ErrorContext(ctx, "video_coords lookup failed", "err", res.Error, "slug", vid.Slug)
		return Moment{}, false
	}
	if res.RowsAffected == 0 {
		return Moment{}, false
	}
	return Moment{
		Lat: row.Lat, Lng: row.Lng,
		State: row.State.String, City: row.City.String, CityM: row.CityM.Float64,
	}, true
}
