package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
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
	if v.PrevVid.Valid {
		d.PrevID = &v.PrevVid.Int64
	}
	if v.NextVid.Valid {
		d.NextID = &v.NextVid.Int64
	}
	d.PrevTrack = neighbourTrack(ctx, db, d.PrevID)
	d.NextTrack = neighbourTrack(ctx, db, d.NextID)
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
