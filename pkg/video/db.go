package video

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/geo"
	"github.com/adanalife/tripbot/pkg/helpers"
	"gorm.io/gorm"
)

// validDashStr matches the 20-character underscore-and-digit timestamp that
// opens every dashcam clip filename.
var validDashStr = regexp.MustCompile(`^[_0-9]{20}$`)

// LoadOrCreate() will look up the video in the DB,
// or add it to the DB if it's not there yet
func LoadOrCreate(ctx context.Context, path string) (Video, error) {
	slug := slug(path)

	vid, err := load(ctx, "slug = ?", slug)
	if err != nil {
		// create a new video
		vid, err = create(ctx, slug)
	}

	return vid, err
}

// load() fetches a Video from the DB by the given GORM conditions: a primary
// key, or a query fragment and its args.
func load(ctx context.Context, conds ...any) (Video, error) {
	var vid Video
	result := database.GormDB().WithContext(ctx).First(&vid, conds...)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return Video{}, errors.New("no matches found")
	}
	return vid, result.Error
}

// create will create a new Video from a slug. The returned Video is the
// inserted row: save() fills in the derived fields, the DB-assigned ID, and
// the autoCreateTime date_created.
func create(ctx context.Context, file string) (Video, error) {
	if file == "" {
		return Video{}, errors.New("no file provided")
	}
	slug := slug(file)

	// validate the dash string
	if err := validate(slug); err != nil {
		return Video{}, err
	}

	// DateCreated is left unset so GORM's autoCreateTime stamps it on insert
	// (see Video struct in type.go).
	newVid := Video{Slug: slug}
	if err := newVid.save(ctx); err != nil {
		slog.ErrorContext(ctx, "error saving to DB", "err", err)
		return Video{}, err
	}
	return newVid, nil
}

// save() fills in the fields derived from the slug and the coords, then
// inserts the video. It writes through the receiver, so a caller holding a
// freshly built Video gets the DB-assigned ID and date_created back.
func (v *Video) save(ctx context.Context) error {
	if v.CoordSource == "" {
		v.CoordSource = CoordSourceOCR
	}

	if v.Lat == 0 || v.Lng == 0 {
		// Nothing runs OCR at runtime, so a clip created here has no GPS fix.
		v.Flagged = true
		v.CoordSource = CoordSourceMissing
	}

	if !v.Flagged {
		// figure out which state we're in
		state, err := geo.State(v.Lat, v.Lng)
		// ErrDisabled is the expected steady-state when no Maps key
		// is configured; don't spam Sentry on every video import.
		if err != nil && !errors.Is(err, geo.ErrDisabled) {
			slog.ErrorContext(ctx, "error geocoding coords", "err", err)
		}
		v.State = state
	}

	v.DateFilmed = v.toDate()
	return database.GormDB().WithContext(ctx).Create(v).Error
}

// NextUnflagged() finds the next unflagged video by walking the next_vid chain.
// The walk is bounded by the playlist length, so a broken chain or a
// cycle of flagged videos returns an error instead of spinning forever.
func (v Video) NextUnflagged(ctx context.Context) (Video, error) {
	var count int64
	if err := database.GormDB().WithContext(ctx).Model(&Video{}).Count(&count).Error; err != nil {
		return Video{}, err
	}

	vid := v
	for i := int64(0); i < count; i++ {
		nextID := vid.NextVid.Int64
		var err error
		vid, err = load(ctx, nextID)
		if err != nil {
			return Video{}, fmt.Errorf("broken next_vid chain at id %d: %w", nextID, err)
		}
		// use the first unflagged video we find
		if !vid.Flagged {
			return vid, nil
		}
	}
	return Video{}, errors.New("no unflagged video found in next_vid chain")
}

func (v Video) SetNextVid(ctx context.Context, nextVid Video) error {
	return database.GormDB().WithContext(ctx).Model(&v).Update("next_vid", nextVid.ID).Error
}

func validate(dashStr string) error {
	if len(dashStr) < 20 {
		return errors.New("dash string too short")
	}
	shortened := dashStr[:20]

	if !validDashStr.MatchString(shortened) {
		return errors.New("dash string did not match regex")
	}
	return nil
}

// randomByStateQuery picks one clip filmed in a state by skipping a random
// number of its rows, so the database reads a single row out of the match set
// rather than sorting the whole set to take the first. The count and the skip
// share one statement, so no clip can be added or removed between them.
const randomByStateQuery = `
	SELECT * FROM videos
	WHERE state = @state
	OFFSET floor(random() * (SELECT count(*) FROM videos WHERE state = @state))
	LIMIT 1`

func FindRandomByState(ctx context.Context, state string) (Video, error) {
	var vid Video

	// convert to long form
	if len(state) == 2 {
		state = helpers.StateAbbrevToState(state)
		if state == "" {
			return vid, fmt.Errorf("unable to parse state abbrev")
		}
	}
	// title-case the state (it's stored in the DB like that)
	state = helpers.TitlecaseState(state)

	result := database.GormDB().WithContext(ctx).Raw(randomByStateQuery, sql.Named("state", state)).Scan(&vid)
	if result.Error != nil {
		slog.ErrorContext(ctx, "error fetching vid from DB", "err", result.Error)
		return vid, result.Error
	}
	if result.RowsAffected == 0 {
		return vid, terrors.ErrNoFootageForState
	}
	return vid, nil
}

// nextDaytimeScanLimit caps how many upcoming clips FindNextDaytime pulls in
// one query. A day of driving is a few hundred one-minute clips, so this is far
// more than enough to reach the following morning even across multi-day gaps in
// the trip, while bounding the scan for a chat command.
const nextDaytimeScanLimit = 5000

// FindNextDaytime returns the first daytime clip filmed on a later local
// calendar day than `after` — the "skip to the next morning" target behind
// !daytime. The dashcam corpus is daytime driving footage, so from a dusk or
// night clip the next daylight is the following day's first daylight clip; this
// walks clips in film order and returns it. Clips without a GPS fix are skipped
// (daytime needs coords to resolve sunrise/sunset). Returns a
// terrors.ErrNoDaytimeFound when no later daytime clip exists in the scanned window.
func FindNextDaytime(ctx context.Context, after Video) (Video, error) {
	// Baseline calendar day: the current clip's local day when it has a fix,
	// else its raw filmed day (a flagged clip has no coords to localize).
	afterDay := time.Date(after.DateFilmed.Year(), after.DateFilmed.Month(), after.DateFilmed.Day(), 0, 0, 0, 0, time.UTC)
	if after.Lat != 0 || after.Lng != 0 {
		afterDay = helpers.LocalDate(after.DateFilmed, after.Lat, after.Lng)
	}

	var clips []Video
	err := database.GormDB().WithContext(ctx).
		Where("date_filmed > ? AND NOT flagged AND (lat != 0 OR lng != 0)", after.DateFilmed).
		Order("date_filmed").
		Limit(nextDaytimeScanLimit).
		Find(&clips).Error
	if err != nil {
		return Video{}, err
	}

	for _, clip := range clips {
		if !helpers.LocalDate(clip.DateFilmed, clip.Lat, clip.Lng).After(afterDay) {
			continue // same (or earlier) local day as the current clip
		}
		if helpers.IsDaytime(clip.DateFilmed, clip.Lat, clip.Lng) {
			return clip, nil
		}
	}
	return Video{}, terrors.ErrNoDaytimeFound
}

// A clip with a per-moment track worth drawing contributes that whole track;
// one without contributes the single fix it has, so a gap in the coords stage's
// coverage bends the route slightly rather than breaking the line. Ordering is
// (film time, offset into the clip) — by clip alone the points inside a clip
// come back in whatever order the scan found them, which draws a scribble.
//
// The confidence rides along so the map can band the line by where each stretch
// came from. It is the clip's, repeated on every one of that clip's points,
// which is what makes a band a contiguous run rather than a per-point property.
const corpusRouteQuery = `
SELECT lat, lng, coord_confidence FROM (
    SELECT c.lat, c.lng, v.coord_confidence, v.date_filmed, c.ts_sec
    FROM video_coords c JOIN videos v ON v.id = c.video_id
    WHERE NOT v.flagged AND v.coord_confidence >= @minConfidence AND c.ts_sec IS NOT NULL
  UNION ALL
    SELECT v.lat, v.lng, v.coord_confidence, v.date_filmed, 0
    FROM videos v
    WHERE NOT v.flagged AND (v.lat != 0 OR v.lng != 0)
      AND (v.coord_confidence IS NULL OR v.coord_confidence < @minConfidence)
) t ORDER BY date_filmed, ts_sec`

// CorpusRoute returns the GPS coordinates of the whole route the van drove,
// ordered along it — the admin map's background-route overlay.
//
// This is the per-moment track, not one point per clip: at clip granularity
// every curve, switchback and cloverleaf of the trip is drawn as a straight
// chord between points ~4 miles apart. Flagged clips and 0/0 are excluded.
// Returns nil on error.
//
// Each point carries the band its clip falls in, so the caller can draw the
// bridged stretches differently from the read ones. A synthetic track is drawn
// because the map wants the road's shape, which it has; it is still far too
// coarse to answer a question with, which is why CoordAt keeps the higher bar.
//
// The result is large — a few hundred thousand points — and callers are
// expected to simplify before serving it. See pkg/server's map handler.
func CorpusRoute(ctx context.Context) []RoutePoint {
	type coord struct {
		Lat             float64
		Lng             float64
		CoordConfidence *float64
	}
	var rows []coord
	err := database.GormDB().WithContext(ctx).
		Raw(corpusRouteQuery, sql.Named("minConfidence", minRouteConfidence)).
		Scan(&rows).Error
	if err != nil {
		slog.ErrorContext(ctx, "corpus route query failed", "err", err)
		return nil
	}
	out := make([]RoutePoint, len(rows))
	for i, r := range rows {
		out[i] = RoutePoint{Lat: r.Lat, Lng: r.Lng, Band: RouteBand(r.CoordConfidence)}
	}
	return out
}
