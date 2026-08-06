package video

import (
	"context"
	"database/sql"
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

// The two ORDER BY booleans do the preference in one pass: Postgres sorts
// false before true, so rows within ocrWindow come first, then within those
// the ones read off the frame, then nearest wins.
const coordAtQuery = `
SELECT lat, lng FROM video_coords
WHERE video_id = @vid AND ts_sec IS NOT NULL AND abs(ts_sec - @ts) <= @window
ORDER BY abs(ts_sec - @ts) > @ocr, source <> 'ocr', abs(ts_sec - @ts)
LIMIT 1`

// CoordAt returns where the van was `at` into vid, from the per-moment track
// in video_coords. ok is false when the clip has no track worth believing, or
// none covering that moment, and the caller falls back to vid.Location() — the
// single clip-level fix, which sits a median 1.3 km from where the van
// actually was.
func CoordAt(ctx context.Context, vid Video, at time.Duration) (lat, lng float64, ok bool) {
	if vid.ID == 0 || vid.CoordConfidence == nil || *vid.CoordConfidence < minCoordConfidence {
		return 0, 0, false
	}
	// A negative offset means the caller's clock disagrees with playout's
	// about which clip is on screen; the top of this one is the closest thing
	// to an answer.
	tsSec := max(at.Seconds(), 0)

	var row struct{ Lat, Lng float64 }
	res := database.GormDB().WithContext(ctx).Raw(coordAtQuery,
		sql.Named("vid", vid.ID),
		sql.Named("ts", tsSec),
		sql.Named("window", coordWindow),
		sql.Named("ocr", ocrWindow),
	).Scan(&row)
	if res.Error != nil {
		slog.ErrorContext(ctx, "video_coords lookup failed", "err", res.Error, "slug", vid.Slug)
		return 0, 0, false
	}
	if res.RowsAffected == 0 {
		return 0, 0, false
	}
	return row.Lat, row.Lng, true
}
