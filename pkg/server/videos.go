package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/bradfitz/latlong"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// videoDetail is one clip's record for the console's video page: the file,
// where and when it was filmed, its neighbours in the corpus, and how often it
// has aired.
type videoDetail struct {
	ID      int      `json:"id"`
	File    string   `json:"file"`
	State   string   `json:"state"`
	City    string   `json:"city"`
	CityM   *float64 `json:"city_m"`
	Place   string   `json:"place"`
	Lat     float64  `json:"lat"`
	Lng     float64  `json:"lng"`
	Flagged bool     `json:"flagged"`
	Corpus  string   `json:"corpus"`
	// RecordedAt and RecordedAtUTC are when the clip was filmed, RFC 3339; see
	// filmedTimes.
	RecordedAt    string `json:"recorded_at"`
	RecordedAtUTC string `json:"recorded_at_utc"`
	PrevID        *int64 `json:"prev_id"`
	NextID        *int64 `json:"next_id"`
	// Plays counts every airing on every platform, all time; LastPlayedAt is
	// the latest one's start, empty when the clip has never aired.
	Plays        int64  `json:"plays"`
	LastPlayedAt string `json:"last_played_at"`
	// Track is the clip's route as [sec, lat, lng] in clip order, and
	// PrevTrack / NextTrack are its neighbours' — the road into and out of it.
	// Each is empty when that clip has no track worth believing. SourceTrack
	// is the route of the whole recording the clip was cut from, sec on the
	// recording's clock, so a trimmed clip's cut stretches can be drawn too.
	Track       [][3]float64 `json:"track"`
	SourceTrack [][3]float64 `json:"source_track"`
	PrevTrack   [][3]float64 `json:"prev_track"`
	NextTrack   [][3]float64 `json:"next_track"`
	// Moments are the operator's marks on the clip, in clip order.
	Moments []moment `json:"moments"`
}

// filmedTimes is when v was filmed, RFC 3339, twice. local is date_filmed as
// stored: the dashcam's own wall clock, as its HUD and file name show it,
// labelled UTC. utc is the real instant: that wall clock read in the time zone
// at the clip's fix. Both are "" when the clip has no date; utc is also ""
// when there is no fix or it falls in no zone.
func filmedTimes(v video.Video) (local, utc string) {
	if v.DateFilmed.IsZero() {
		return "", ""
	}
	t := v.DateFilmed.UTC()
	local = t.Format(time.RFC3339)
	if v.Lat == 0 && v.Lng == 0 {
		return local, ""
	}
	zone := latlong.LookupZoneName(v.Lat, v.Lng)
	if zone == "" {
		return local, ""
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return local, ""
	}
	return local, time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc).
		UTC().Format(time.RFC3339)
}

// trackOf is vid's track as triples, never nil, so it encodes as [].
func trackOf(ctx context.Context, vid video.Video) [][3]float64 {
	return triples(clipTrack(ctx, vid))
}

// sourceTrackOf is vid's whole recording's track as triples, never nil.
func sourceTrackOf(ctx context.Context, vid video.Video) [][3]float64 {
	return triples(clipSourceTrack(ctx, vid))
}

func triples(pts []video.TrackPoint) [][3]float64 {
	out := [][3]float64{}
	for _, p := range pts {
		out = append(out, [3]float64{p.Sec, p.Lat, p.Lng})
	}
	return out
}

// neighbourTrack is the track of the clip id names, empty when there is no
// neighbour or it can't be read: a missing side road is not worth a 500.
func neighbourTrack(ctx context.Context, db *gorm.DB, id *int64) [][3]float64 {
	var v video.Video
	if id == nil || db.First(&v, *id).Error != nil {
		return [][3]float64{}
	}
	return trackOf(ctx, v)
}

// ponytail: video_plays is indexed by (platform, started_at), so this scans
// it — one operator click, not a hot path. An index on video_id if it shows.
const videoPlaysSQL = `
SELECT COUNT(*) AS plays, MAX(started_at) AS last
FROM video_plays WHERE video_id = @id`

func videoByID(ctx context.Context, id int) (videoDetail, error) {
	db := database.GormDB().WithContext(ctx)
	var v video.Video
	if err := db.First(&v, id).Error; err != nil {
		return videoDetail{}, err
	}
	var plays struct {
		Plays int64
		Last  sql.NullTime
	}
	if err := db.Raw(videoPlaysSQL, sql.Named("id", id)).Scan(&plays).Error; err != nil {
		return videoDetail{}, err
	}
	d := videoDetail{
		ID: v.ID, File: v.File(), State: v.State, City: v.City, CityM: v.CityM,
		Place: v.Place(), Lat: v.Lat, Lng: v.Lng, Flagged: v.Flagged, Corpus: v.Corpus,
		Plays: plays.Plays, Track: trackOf(ctx, v), SourceTrack: sourceTrackOf(ctx, v),
	}
	d.RecordedAt, d.RecordedAtUTC = filmedTimes(v)
	// The clip before is the one whose next_vid names this one: the chain is
	// only ever written forward, so the prev_vid column stays empty.
	// ponytail: next_vid is unindexed, so this scans videos — one operator
	// click over a few thousand rows. An index on next_vid if it shows.
	var prev []int64
	if err := db.Model(&video.Video{}).Where("next_vid = ?", id).Limit(1).Pluck("id", &prev).Error; err != nil {
		return videoDetail{}, err
	}
	if len(prev) > 0 {
		d.PrevID = &prev[0]
	}
	if v.NextVid.Valid {
		d.NextID = &v.NextVid.Int64
	}
	d.PrevTrack = neighbourTrack(ctx, db, d.PrevID)
	d.NextTrack = neighbourTrack(ctx, db, d.NextID)
	moments, err := momentsOf(ctx, db, id)
	if err != nil {
		return videoDetail{}, err
	}
	d.Moments = moments
	if plays.Last.Valid {
		d.LastPlayedAt = plays.Last.Time.UTC().Format(time.RFC3339)
	}
	return d, nil
}

// videoHandler serves GET /api/videos/{id}: one clip's record, 404 when no
// clip has that id.
func videoHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || id <= 0 {
		http.Error(w, `{"error":"bad video id"}`, http.StatusBadRequest)
		return
	}
	d, err := videoByID(r.Context(), id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, `{"error":"no such video"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		logQueryFailure(r.Context(), "video lookup failed", err, "id", id)
		http.Error(w, `{"error":"couldn't read video"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(d); err != nil {
		logQueryFailure(r.Context(), "video encode failed", err)
	}
}

// s2Stem is a season-2 original's file stem — the camera's 14-digit clock and
// sequence — which every piece cut from it carries as its slug's prefix.
var s2Stem = regexp.MustCompile(`^\d{14}_\d{6}$`)

// sourceClip is one piece of an original, as GET /api/sources/{stem} lists
// them: its fix, place, filming times (as videoDetail's) and track ([sec,
// lat, lng] like a clip's), so a page can draw the whole original's route
// without asking per clip.
type sourceClip struct {
	ID            int          `json:"id"`
	Slug          string       `json:"slug"`
	Lat           float64      `json:"lat"`
	Lng           float64      `json:"lng"`
	Place         string       `json:"place"`
	RecordedAt    string       `json:"recorded_at"`
	RecordedAtUTC string       `json:"recorded_at_utc"`
	Track         [][3]float64 `json:"track"`
	SourceTrack   [][3]float64 `json:"source_track"`
}

// withTracks lists clips in the order given, each with its fix and track.
func withTracks(ctx context.Context, vids []video.Video) []sourceClip {
	clips := make([]sourceClip, 0, len(vids))
	for _, v := range vids {
		c := sourceClip{
			ID: v.ID, Slug: v.Slug, Lat: v.Lat, Lng: v.Lng, Place: v.Place(),
			Track: trackOf(ctx, v), SourceTrack: sourceTrackOf(ctx, v),
		}
		c.RecordedAt, c.RecordedAtUTC = filmedTimes(v)
		clips = append(clips, c)
	}
	return clips
}

// sourceCut is one piece of an original as auto-trim proposed it and, once
// someone has, as the reviewer ruled it — video_cuts' row, passed through as
// the pipeline wrote it (the proposal and ruling shapes are video-pipeline's).
// Track is a piece with no clip's own track, [[sec, lat, lng], ...] on the
// piece's clock, and [] when it has none worth believing; a piece that aired
// has its track on its clip instead.
type sourceCut struct {
	Slug     string          `json:"slug"`
	Proposal json.RawMessage `json:"proposal"`
	Ruling   json.RawMessage `json:"ruling"`
	Track    json.RawMessage `json:"track"`
}

const sourceCutsSQL = `SELECT c.slug, c.proposal, c.ruling, COALESCE(t.points, '[]'::jsonb) AS track
FROM video_cuts c
LEFT JOIN piece_tracks t ON t.slug = c.slug AND t.confidence >= @min
WHERE c.slug LIKE @pattern ORDER BY c.slug`

// sourceResponse is the wire shape of GET /api/sources/{stem}: the clips the
// pipeline ingested from that original, in offset order, the ingested
// originals either side of it in film order ("" at either end), the corpus
// its clips share, every airing of any of them on any platform, and every
// piece's proposed cuts, dropped pieces included.
type sourceResponse struct {
	Stem   string       `json:"stem"`
	Corpus string       `json:"corpus"`
	Clips  []sourceClip `json:"clips"`
	Plays  int64        `json:"plays"`
	Prev   string       `json:"prev"`
	Next   string       `json:"next"`
	Cuts   []sourceCut  `json:"cuts"`
}

// ponytail: like videoPlaysSQL, a scan of video_plays — one page view, not a
// hot path. An index on video_id if it shows.
const sourcePlaysSQL = `SELECT COUNT(*) FROM video_plays WHERE video_id IN @ids`

// neighbourSourceSQL finds the nearest other ingested original before or after
// a stem. A stem leads with the camera's clock, so slug order is film order;
// left(slug, 21) is a piece's stem. ponytail: a scan of videos per page view,
// a few thousand rows; an expression index on left(slug, 21) if it shows.
const neighbourSourceSQL = `SELECT left(slug, 21) FROM videos
WHERE slug ~ '^[0-9]{14}_[0-9]{6}_s' AND left(slug, 21) %s @stem
ORDER BY slug %s LIMIT 1`

func neighbourSource(ctx context.Context, stem string, after bool) (string, error) {
	op, dir := "<", "DESC"
	if after {
		op, dir = ">", "ASC"
	}
	var out []string
	err := database.GormDB().WithContext(ctx).
		Raw(fmt.Sprintf(neighbourSourceSQL, op, dir), sql.Named("stem", stem)).
		Scan(&out).Error
	if err != nil || len(out) == 0 {
		return "", err
	}
	return out[0], nil
}

// sourceHandler serves GET /api/sources/{stem}: the clips cut from one
// original and its pieces' proposed cuts, so the console's pipeline grid can
// open a source once it has been ingested. 404 until the first piece lands or
// the pipeline records its cuts.
func sourceHandler(w http.ResponseWriter, r *http.Request) {
	stem := mux.Vars(r)["stem"]
	if !s2Stem.MatchString(stem) {
		http.Error(w, `{"error":"bad source stem"}`, http.StatusBadRequest)
		return
	}
	// The stem's own underscore is escaped so LIKE matches it literally: the
	// same pattern the pipeline's ingest reconciles a source's rows with.
	pattern := strings.ReplaceAll(stem, "_", `\_`) + `\_s%`
	var vids []video.Video
	err := database.GormDB().WithContext(r.Context()).
		Where("slug LIKE ?", pattern).Order("slug").Find(&vids).Error
	clips := withTracks(r.Context(), vids)
	cuts := []sourceCut{}
	if err == nil {
		err = database.GormDB().WithContext(r.Context()).
			Raw(sourceCutsSQL, sql.Named("pattern", pattern), sql.Named("min", video.MinCoordConfidence)).
			Scan(&cuts).Error
	}
	if err != nil {
		logQueryFailure(r.Context(), "source lookup failed", err, "stem", stem)
		http.Error(w, `{"error":"couldn't read source"}`, http.StatusInternalServerError)
		return
	}
	if len(clips) == 0 && len(cuts) == 0 {
		http.Error(w, `{"error":"no clips from that source"}`, http.StatusNotFound)
		return
	}
	resp := sourceResponse{Stem: stem, Clips: clips, Cuts: cuts}
	if len(vids) > 0 {
		resp.Corpus = vids[0].Corpus
	}
	ids := make([]int, len(vids))
	for i, v := range vids {
		ids[i] = v.ID
	}
	err = database.GormDB().WithContext(r.Context()).
		Raw(sourcePlaysSQL, sql.Named("ids", ids)).Scan(&resp.Plays).Error
	if err == nil {
		resp.Prev, err = neighbourSource(r.Context(), stem, false)
	}
	if err == nil {
		resp.Next, err = neighbourSource(r.Context(), stem, true)
	}
	if err != nil {
		logQueryFailure(r.Context(), "source plays or neighbours failed", err, "stem", stem)
		http.Error(w, `{"error":"couldn't read source"}`, http.StatusInternalServerError)
		return
	}
	writeInsights(w, r, resp)
}

// footageDaySQL is the day a clip was filmed, off its slug: both cameras name
// files by their own clock (season 2 `20260307154839_…`, season 1
// `2018_0514_224801_…`), so this is the day the operator reads in the file
// name — the same wall clock date_filmed stores, read without a zone so a
// piece dated by its own first frame still lands on its original's day. NULL
// for a slug of neither shape.
const footageDaySQL = `CASE
  WHEN slug ~ '^[0-9]{14}_' THEN to_date(left(slug, 8), 'YYYYMMDD')
  WHEN slug ~ '^[0-9]{4}_[0-9]{4}_' THEN to_date(left(slug, 9), 'YYYY_MMDD')
END`

// footageDay is one filmed day and how many clips it holds.
type footageDay struct {
	Day   string `json:"day"`
	Clips int    `json:"clips"`
}

// droppedCut is one piece dropped whole, as GET /api/footage/dropped lists
// them: its cut row, and the clip it aired as (0 when it never did — a piece a
// reviewer kept after auto-trim proposed the drop has one).
type droppedCut struct {
	Slug     string          `json:"slug"`
	Proposal json.RawMessage `json:"proposal"`
	Ruling   json.RawMessage `json:"ruling"`
	VideoID  int             `json:"video_id"`
}

// droppedSQL lists the pieces either auto-trim or a reviewer ruled out whole,
// newest footage first (a slug leads with the camera's clock).
// ponytail: a scan of video_cuts per call, a row per season-2 piece; an
// expression index on the verdicts if it shows.
const droppedSQL = `SELECT c.slug, c.proposal, c.ruling, COALESCE(v.id, 0) AS video_id
FROM video_cuts c LEFT JOIN videos v ON v.slug = c.slug
WHERE c.proposal->>'verdict' = 'drop' OR c.ruling->>'verdict' = 'drop'
ORDER BY c.slug DESC`

// footageDroppedHandler serves GET /api/footage/dropped: every season-2 piece
// dropped whole by auto-trim's proposal or a reviewer's ruling, for the
// console's dropped-clips page.
func footageDroppedHandler(w http.ResponseWriter, r *http.Request) {
	cuts := []droppedCut{}
	if err := database.GormDB().WithContext(r.Context()).Raw(droppedSQL).Scan(&cuts).Error; err != nil {
		logQueryFailure(r.Context(), "dropped pieces failed", err)
		http.Error(w, `{"error":"couldn't read dropped pieces"}`, http.StatusInternalServerError)
		return
	}
	writeInsights(w, r, cuts)
}

// footageDaysHandler serves GET /api/footage/days: every day with footage,
// oldest first, for the console's calendar.
// ponytail: a scan of videos per call, a few thousand rows.
func footageDaysHandler(w http.ResponseWriter, r *http.Request) {
	days := []footageDay{}
	err := database.GormDB().WithContext(r.Context()).Raw(`SELECT to_char(day, 'YYYY-MM-DD') AS day, count(*) AS clips
FROM (SELECT ` + footageDaySQL + ` AS day FROM videos) d
WHERE day IS NOT NULL GROUP BY day ORDER BY day`).Scan(&days).Error
	if err != nil {
		logQueryFailure(r.Context(), "footage days failed", err)
		http.Error(w, `{"error":"couldn't read footage days"}`, http.StatusInternalServerError)
		return
	}
	writeInsights(w, r, days)
}

// footageDayResponse is the wire shape of GET /api/footage/days/{day}.
type footageDayResponse struct {
	Day   string       `json:"day"`
	Clips []sourceClip `json:"clips"`
}

// footageDayHandler serves GET /api/footage/days/{day}: the clips filmed that
// day in film order, each with its fix and track. 404 for a day with none.
func footageDayHandler(w http.ResponseWriter, r *http.Request) {
	day := mux.Vars(r)["day"]
	if _, err := time.Parse(time.DateOnly, day); err != nil {
		http.Error(w, `{"error":"bad day"}`, http.StatusBadRequest)
		return
	}
	var vids []video.Video
	err := database.GormDB().WithContext(r.Context()).
		Where(footageDaySQL+" = ?::date", day).Order("slug").Find(&vids).Error
	if err != nil {
		logQueryFailure(r.Context(), "footage day failed", err, "day", day)
		http.Error(w, `{"error":"couldn't read footage day"}`, http.StatusInternalServerError)
		return
	}
	if len(vids) == 0 {
		http.Error(w, `{"error":"no footage that day"}`, http.StatusNotFound)
		return
	}
	writeInsights(w, r, footageDayResponse{Day: day, Clips: withTracks(r.Context(), vids)})
}
