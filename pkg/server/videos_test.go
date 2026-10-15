package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/gorilla/mux"
)

func videoGET(t *testing.T, id string) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/videos/{id}", http.HandlerFunc(videoHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+id, nil))
	return rec
}

// One clip's record: its place, its neighbours and their tracks, and every
// airing on every platform, counted.
func TestVideoHandler(t *testing.T) {
	db := testdb.New(t)
	// Clips 6 → 7 → 8 are chained by next_vid alone, as the corpus is: clip 7
	// finds 6 as the clip whose next_vid names it. Clip 6 has no track, so its
	// side reads empty rather than failing the page.
	orig := clipTrack
	t.Cleanup(func() { clipTrack = orig })
	clipTrack = func(_ context.Context, v video.Video) []video.TrackPoint {
		switch v.ID {
		case 7:
			return []video.TrackPoint{{Sec: 0, Lat: 38.5, Lng: -109.5}, {Sec: 30, Lat: 38.6, Lng: -109.6}}
		case 8:
			return []video.TrackPoint{{Sec: 0, Lat: 38.7, Lng: -109.7}}
		}
		return nil
	}
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO videos (id, slug, state, lat, lng, next_vid) VALUES (6, '2018_0514_224501_012', 'Utah', 38.5, -109.5, 7)`,
		`INSERT INTO videos (id, slug, state, city, city_m, lat, lng, next_vid, date_filmed)
		 VALUES (7, '2018_0514_224801_013', 'Utah', 'Moab', 0, 38.57, -109.55, 8, '2018-05-14 22:48:01+00')`,
		`INSERT INTO videos (id, slug, state, lat, lng) VALUES (8, '2018_0514_225101_014', 'Utah', 38.7, -109.7)`,
		`INSERT INTO video_plays (platform, video_id, started_at) VALUES
		 ('twitch', 7, '2026-10-01 00:00:00+00'), ('youtube', 7, '2026-10-02 00:00:00+00'), ('twitch', 9, now())`,
	} {
		if err := db.WithContext(ctx).Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}

	rec := videoGET(t, "7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got videoDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	zero, prev, next := 0.0, int64(6), int64(8)
	want := videoDetail{
		ID: 7, File: "2018_0514_224801_013.MP4", State: "Utah", City: "Moab", CityM: &zero,
		Place: "Moab, Utah", Lat: 38.57, Lng: -109.55, Corpus: "s1",
		RecordedAt: "2018-05-14T22:48:01Z", PrevID: &prev, NextID: &next,
		Plays: 2, LastPlayedAt: "2026-10-02T00:00:00Z",
		Track:     [][3]float64{{0, 38.5, -109.5}, {30, 38.6, -109.6}},
		PrevTrack: [][3]float64{},
		NextTrack: [][3]float64{{0, 38.7, -109.7}},
		Moments:   []moment{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestVideoHandler_Misses(t *testing.T) {
	testdb.New(t)
	if rec := videoGET(t, "404"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: status = %d, want 404", rec.Code)
	}
	if rec := videoGET(t, "abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("non-numeric id: status = %d, want 400", rec.Code)
	}
}

func momentPOST(t *testing.T, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/videos/{id}/moments", http.HandlerFunc(videoMomentsHandler)).Methods("POST")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/videos/"+id+"/moments", strings.NewReader(body)))
	return rec
}

// A mark is stored on the original recording's clock and read back on the
// clip's: clip 7 is a trim starting 15s into its original, so a star at 30s
// into the clip lands at 45s and reads back at 30s. Marking it again replaces
// its note; removing it takes it off.
func TestVideoMomentsHandler(t *testing.T) {
	db := testdb.New(t)
	for _, q := range []string{
		`INSERT INTO videos (id, slug, state, lat, lng) VALUES (7, '2018_0514_224801_013', 'Utah', 38.57, -109.55)`,
		`INSERT INTO video_coords (video_id, ts_sec, source_ts_sec, lat, lng, source) VALUES (7, 10, 25, 38.5, -109.5, 'ocr')`,
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	moments := func() []moment {
		t.Helper()
		got, err := momentsOf(context.Background(), db, 7)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	for _, body := range []string{
		`{"kind":"star","sec":30}`,
		`{"kind":"star","sec":30,"note":"the canyon opens up"}`,
		`{"kind":"bad_coord","sec":10.1}`,
	} {
		if rec := momentPOST(t, "7", body); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", body, rec.Code, rec.Body.String())
		}
	}
	var src float64
	db.Raw(`SELECT source_ts_sec FROM video_moments WHERE kind = 'star'`).Scan(&src)
	if src != 45 {
		t.Errorf("stored source_ts_sec = %v, want 45", src)
	}
	got := moments()
	if len(got) != 2 || got[0].Kind != "bad_coord" || math.Abs(got[0].Sec-10.1) > 0.01 ||
		got[1] != (moment{Kind: "star", Sec: 30, Note: "the canyon opens up"}) {
		t.Errorf("got %+v", got)
	}

	// A fractional second read back (REAL, so not exactly 10.1) still
	// removes its mark.
	for _, body := range []string{
		`{"kind":"star","sec":30,"remove":true}`,
		fmt.Sprintf(`{"kind":"bad_coord","sec":%v,"remove":true}`, got[0].Sec),
	} {
		if rec := momentPOST(t, "7", body); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", body, rec.Code)
		}
	}
	if got := moments(); len(got) != 0 {
		t.Errorf("after remove got %+v, want none", got)
	}

	for body, code := range map[string]int{
		`{"kind":"nope","sec":1}`:  http.StatusBadRequest,
		`{"kind":"star"}`:          http.StatusBadRequest,
		`{"kind":"star","sec":-1}`: http.StatusBadRequest,
	} {
		if rec := momentPOST(t, "7", body); rec.Code != code {
			t.Errorf("%s: status = %d, want %d", body, rec.Code, code)
		}
	}
	if rec := momentPOST(t, "99", `{"kind":"star","sec":1}`); rec.Code != http.StatusNotFound {
		t.Errorf("missing clip: status = %d, want 404", rec.Code)
	}
}
