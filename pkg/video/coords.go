package video

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
)

// MinCoordConfidence is how believable a clip's per-moment track has to be
// before a moment inside it is worth answering with. Interpolation always
// produces rows, so a clip whose overlay mostly failed to read still has a
// full track — videos.coord_confidence is the only thing that says so, and
// below this the clip's single fix is the more honest answer. 0.8 keeps 4,023
// of the corpus's 4,354 clips (measured 2026-08-05).
const MinCoordConfidence = 0.8

// coordWindow is how far either side of a moment, in seconds, a coordinate may
// be read from. The track is sampled on a 2 s grid, so a row this close is the
// same stretch of road; past it the track has a gap and the clip's single fix
// beats extrapolating across it.
const coordWindow = 10.0

// ocrWindow is how far from the moment asked about, in seconds, a row read off
// the frame may be and still beat a nearer interpolated one. Inside it both
// rows describe the same few meters of road and the measured one is worth
// more; outside it, proximity wins.
const ocrWindow = 1.5

// Route bands name where a stretch of the drawn route got its coordinates, so
// the map can say which parts are read and which are inferred. They come from
// the two thresholds the pipeline already writes against, not from a new scale:
// 0.8 is the trusted gate every consumer applies, and 0.5 is exactly what the
// synth pass stamps on a clip it bridged from its neighbors.
const (
	BandTrusted   = "trusted"   // the clip's own OCR track, believed
	BandSynthetic = "synthetic" // interpolated across the clip from trusted neighbors
	BandUntrusted = "untrusted" // no track worth drawing; one point for the whole clip
)

// minRouteConfidence is how believable a track has to be to be drawn per-moment
// rather than as a single point. Lower than MinCoordConfidence on purpose: a
// bridged track is far too coarse to answer "where are we" with, but it is
// still the real shape of the road, and the alternative on the map is a
// straight chord between clips ~4 miles apart.
const minRouteConfidence = SynthConfidence

// SynthConfidence is what video-pipeline's synth pass writes on a clip whose
// track it bridged. Named here because the map keys a band off it; the value
// is owned by the pipeline, not by tripbot.
const SynthConfidence = 0.5

// RoutePoint is one point of the drawn route, tagged with where its clip's
// coordinates came from and which season of the trip filmed it.
type RoutePoint struct {
	Lat, Lng float64
	Band     string
	Season   string
}

// Season names the trip a corpus belongs to: "s1" (2018) or "s2" (2026).
// s2fast is season 2 shot in the camera's time-lapse mode, so it folds into s2.
func Season(corpus string) string {
	if corpus == CorpusS2 || corpus == CorpusS2Fast {
		return CorpusS2
	}
	return CorpusS1
}

// RouteBand names the confidence tier a clip's coordinates come from.
func RouteBand(confidence *float64) string {
	switch {
	case confidence == nil:
		return BandUntrusted
	case *confidence >= MinCoordConfidence:
		return BandTrusted
	case *confidence >= minRouteConfidence:
		return BandSynthetic
	}
	return BandUntrusted
}

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
	// CityM is how far the coordinate is from City, in meters; 0 means inside
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

// Place renders the clip's own location the way Moment.Place renders a
// playhead one, so a clip answering from its representative coordinate reads
// identically to one answering from the track. Empty until the geocode pass has
// named the row, which is the caller's signal to fall back further.
//
// A city without a distance can't be rendered honestly — "Bishop, California"
// and "Somewhere in California" are 100 km apart — so a half-filled row degrades
// to the state rather than claiming the nearest town is the current one.
func (v Video) Place() string {
	m := Moment{State: v.State}
	if v.City != "" && v.CityM != nil {
		m.City, m.CityM = v.City, *v.CityM
	}
	return m.Place()
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
	if vid.ID == 0 || vid.CoordConfidence == nil || *vid.CoordConfidence < MinCoordConfidence {
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

// TrackPoint is one moment of a clip's track: Sec into the clip, and where the
// van was then.
type TrackPoint struct {
	Sec, Lat, Lng float64
}

// One row per moment of the source recording (the primary key), so the aired
// rows are already one per moment of the clip; null ts_sec rows lie outside
// the trim.
const tracksQuery = `
SELECT video_id, ts_sec, source_ts_sec, lat, lng FROM video_coords
WHERE video_id IN @ids
ORDER BY video_id, source_ts_sec`

// Tracks reads many clips' tracks in one query, keyed by clip ID: track is
// each clip's aired moments in clip order, source the whole recording it was
// cut from (cut seconds included) on the recording's own clock. Gated like
// CoordAt: a clip with no track worth believing has no entry, which is the
// caller's signal to fall back to the clip-level fix.
func Tracks(ctx context.Context, vids []Video) (track, source map[int][]TrackPoint) {
	var ids []int
	for _, v := range vids {
		if v.ID != 0 && v.CoordConfidence != nil && *v.CoordConfidence >= MinCoordConfidence {
			ids = append(ids, v.ID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []struct {
		VideoID     int
		TsSec       sql.NullFloat64
		SourceTsSec float64
		Lat, Lng    float64
	}
	if err := database.GormDB().WithContext(ctx).Raw(tracksQuery, sql.Named("ids", ids)).Scan(&rows).Error; err != nil {
		slog.ErrorContext(ctx, "video_coords track lookup failed", "err", err, "clips", len(ids))
		return nil, nil
	}
	track, source = map[int][]TrackPoint{}, map[int][]TrackPoint{}
	for _, r := range rows {
		source[r.VideoID] = append(source[r.VideoID], TrackPoint{r.SourceTsSec, r.Lat, r.Lng})
		if r.TsSec.Valid {
			track[r.VideoID] = append(track[r.VideoID], TrackPoint{r.TsSec.Float64, r.Lat, r.Lng})
		}
	}
	for _, pts := range track {
		slices.SortStableFunc(pts, func(a, b TrackPoint) int { return cmp.Compare(a.Sec, b.Sec) })
	}
	return track, source
}

// Track returns vid's whole per-moment track in clip order, nil when the clip
// has no track worth believing.
func Track(ctx context.Context, vid Video) []TrackPoint {
	track, _ := Tracks(ctx, []Video{vid})
	return track[vid.ID]
}

// SourceTrack is Track over the recording the clip was cut from, Sec on the
// recording's clock: the same points for an untrimmed clip, the cut stretches
// too for a trimmed one.
func SourceTrack(ctx context.Context, vid Video) []TrackPoint {
	_, source := Tracks(ctx, []Video{vid})
	return source[vid.ID]
}
