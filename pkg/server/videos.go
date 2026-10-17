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
	// RecordedAt is when the dashcam filmed the clip, RFC 3339; empty when
	// the slug didn't yield a date.
	RecordedAt string `json:"recorded_at"`
	PrevID     *int64 `json:"prev_id"`
	NextID     *int64 `json:"next_id"`
	// Plays counts every airing on every platform, all time; LastPlayedAt is
	// the latest one's start, empty when the clip has never aired.
	Plays        int64  `json:"plays"`
	LastPlayedAt string `json:"last_played_at"`
	// Track is the clip's route as [sec, lat, lng] in clip order, and
	// PrevTrack / NextTrack are its neighbours' — the road into and out of it.
	// Each is empty when that clip has no track worth believing.
	Track     [][3]float64 `json:"track"`
	PrevTrack [][3]float64 `json:"prev_track"`
	NextTrack [][3]float64 `json:"next_track"`
	// Moments are the operator's marks on the clip, in clip order.
	Moments []moment `json:"moments"`
}

// trackOf is vid's track as triples, never nil, so it encodes as [].
func trackOf(ctx context.Context, vid video.Video) [][3]float64 {
	out := [][3]float64{}
	for _, p := range clipTrack(ctx, vid) {
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
		Plays: plays.Plays, Track: trackOf(ctx, v),
	}
	if !v.DateFilmed.IsZero() {
		d.RecordedAt = v.DateFilmed.UTC().Format(time.RFC3339)
	}
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
// them: its fix and its track ([sec, lat, lng] like a clip's), so a page can
// draw the whole original's route without asking per clip.
type sourceClip struct {
	ID    int          `json:"id"`
	Slug  string       `json:"slug"`
	Lat   float64      `json:"lat"`
	Lng   float64      `json:"lng"`
	Track [][3]float64 `json:"track"`
}

// withTracks lists clips in the order given, each with its fix and track.
func withTracks(ctx context.Context, vids []video.Video) []sourceClip {
	clips := make([]sourceClip, 0, len(vids))
	for _, v := range vids {
		clips = append(clips, sourceClip{
			ID: v.ID, Slug: v.Slug, Lat: v.Lat, Lng: v.Lng, Track: trackOf(ctx, v),
		})
	}
	return clips
}

// sourceResponse is the wire shape of GET /api/sources/{stem}: the clips the
// pipeline ingested from that original, in offset order, and the ingested
// originals either side of it in film order ("" at either end).
type sourceResponse struct {
	Stem  string       `json:"stem"`
	Clips []sourceClip `json:"clips"`
	Prev  string       `json:"prev"`
	Next  string       `json:"next"`
}

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
// original, so the console's pipeline grid can open a source once it has
// been ingested. 404 until the first piece lands.
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
	if err != nil {
		logQueryFailure(r.Context(), "source lookup failed", err, "stem", stem)
		http.Error(w, `{"error":"couldn't read source"}`, http.StatusInternalServerError)
		return
	}
	if len(clips) == 0 {
		http.Error(w, `{"error":"no clips from that source"}`, http.StatusNotFound)
		return
	}
	resp := sourceResponse{Stem: stem, Clips: clips}
	if resp.Prev, err = neighbourSource(r.Context(), stem, false); err == nil {
		resp.Next, err = neighbourSource(r.Context(), stem, true)
	}
	if err != nil {
		logQueryFailure(r.Context(), "source neighbours failed", err, "stem", stem)
		http.Error(w, `{"error":"couldn't read source"}`, http.StatusInternalServerError)
		return
	}
	writeInsights(w, r, resp)
}

// footageDaySQL is the day a clip was filmed, off its slug: both cameras name
// files by their own clock (season 2 `20260307154839_…`, season 1
// `2018_0514_224801_…`), so this is the local day the operator reads in the
// file name, where date_filmed is UTC and would split an evening drive at
// midnight. NULL for a slug of neither shape.
const footageDaySQL = `CASE
  WHEN slug ~ '^[0-9]{14}_' THEN to_date(left(slug, 8), 'YYYYMMDD')
  WHEN slug ~ '^[0-9]{4}_[0-9]{4}_' THEN to_date(left(slug, 9), 'YYYY_MMDD')
END`

// footageDay is one filmed day and how many clips it holds.
type footageDay struct {
	Day   string `json:"day"`
	Clips int    `json:"clips"`
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
