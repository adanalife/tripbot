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

// One clip's record: its place, its neighbours and their tracks, every airing
// on every platform, counted, and its filming time both as the dashcam's clock
// read it and as the real UTC instant — 22:48 in Moab in May is MDT, UTC-6.
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
		RecordedAt: "2018-05-14T22:48:01Z", RecordedAtUTC: "2018-05-15T04:48:01Z",
		PrevID: &prev, NextID: &next,
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

func sourceGET(t *testing.T, stem string) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/sources/{stem}", http.HandlerFunc(sourceHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sources/"+stem, nil))
	return rec
}

// One original's pieces, in offset order, with the ingested originals either
// side in film order; a stem that differs by one digit doesn't match through
// the LIKE wildcard, and an original with nothing ingested yet is a 404. The
// source carries its corpus and every airing of its own clips; each clip its
// place and filming times, where a clip with no fix has no real-UTC time.
func TestSourceHandler(t *testing.T) {
	db := testdb.New(t)
	orig := clipTrack
	t.Cleanup(func() { clipTrack = orig })
	clipTrack = func(_ context.Context, v video.Video) []video.TrackPoint {
		if v.ID == 3 {
			return []video.TrackPoint{{Sec: 0, Lat: 35.1, Lng: -106.6}, {Sec: 20, Lat: 35.2, Lng: -106.7}}
		}
		return nil
	}
	for _, q := range []string{
		`INSERT INTO videos (id, slug, corpus, lat, lng, state, city, city_m, date_filmed)
		 VALUES (3, '20260501120000_000123_s0120', 's2', 35.1, -106.6, 'New Mexico', 'Albuquerque', 0, '2026-05-01 12:02:00+00')`,
		`INSERT INTO videos (id, slug, corpus, date_filmed) VALUES (2, '20260501120000_000123_s0000', 's2', '2026-05-01 12:00:00+00')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (4, '20260501120000_010123_s0000', 's2')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (5, '20260430090000_000001_s0300', 's2fast')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (6, '20260430090000_000001_s0000', 's2fast')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (7, '2018_0514_224801_013', 's1')`,
		`INSERT INTO video_plays (platform, video_id, started_at) VALUES
		 ('twitch', 2, now()), ('twitch', 3, now()), ('youtube', 3, now()), ('twitch', 4, now())`,
		`INSERT INTO video_cuts (slug, proposal, ruling) VALUES
		 ('20260501120000_000123_s0000', '{"verdict":"keep"}', NULL),
		 ('20260501120000_000123_s0300', '{"verdict":"drop"}', '{"verdict":"drop"}'),
		 ('20260502080000_000124_s0000', '{"verdict":"drop"}', NULL)`,
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	rec := sourceGET(t, "20260501120000_000123")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := sourceResponse{Stem: "20260501120000_000123", Corpus: "s2", Clips: []sourceClip{
		{ID: 2, Slug: "20260501120000_000123_s0000", RecordedAt: "2026-05-01T12:00:00Z", Track: [][3]float64{}},
		{ID: 3, Slug: "20260501120000_000123_s0120", Lat: 35.1, Lng: -106.6,
			Place: "Albuquerque, New Mexico", RecordedAt: "2026-05-01T12:02:00Z", RecordedAtUTC: "2026-05-01T18:02:00Z",
			Track: [][3]float64{{0, 35.1, -106.6}, {20, 35.2, -106.7}}},
	}, Plays: 3, Prev: "20260430090000_000001", Next: "20260501120000_010123"}
	// A dropped piece has a cut row and no clip; the cuts are checked apart
	// because jsonb hands back its own spacing.
	cuts := got.Cuts
	got.Cuts = nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
	if len(cuts) != 2 || cuts[0].Slug != "20260501120000_000123_s0000" || cuts[1].Slug != "20260501120000_000123_s0300" {
		t.Fatalf("cuts = %+v", cuts)
	}
	if string(cuts[0].Ruling) != "null" || !strings.Contains(string(cuts[1].Ruling), `"drop"`) {
		t.Errorf("rulings = %s, %s", cuts[0].Ruling, cuts[1].Ruling)
	}
	// Every piece of a source dropped: no clips, still its cuts.
	if rec := sourceGET(t, "20260502080000_000124"); rec.Code != http.StatusOK {
		t.Errorf("all-dropped source: status = %d, want 200", rec.Code)
	}
	// The first original has nothing before it; season 1 is never a neighbour.
	rec = sourceGET(t, "20260430090000_000001")
	got = sourceResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Prev != "" || got.Next != "20260501120000_000123" {
		t.Errorf("first source: prev %q next %q", got.Prev, got.Next)
	}
	if rec := sourceGET(t, "20260501120000_999999"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown stem: status = %d, want 404", rec.Code)
	}
	if rec := sourceGET(t, "2018_0514_224801_013"); rec.Code != http.StatusBadRequest {
		t.Errorf("season-1 slug: status = %d, want 400", rec.Code)
	}
}

func footageGET(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/footage/days", http.HandlerFunc(footageDaysHandler)).Methods("GET")
	r.Handle("/api/footage/days/{day}", http.HandlerFunc(footageDayHandler)).Methods("GET")
	r.Handle("/api/footage/dropped", http.HandlerFunc(footageDroppedHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// Days come off the slug's own clock for both cameras, so a season-2 clip at
// 23:30 on the camera stays on its day whatever date_filmed says in UTC; a
// slug of neither shape is on no day.
func TestFootageDays(t *testing.T) {
	db := testdb.New(t)
	for _, q := range []string{
		`INSERT INTO videos (id, slug, corpus, date_filmed) VALUES (2, '20260307233000_000009_s0120', 's2', '2026-03-08 06:30:00+00')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (1, '20260307154839_000001_s0000', 's2')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (3, '2018_0514_224801_013', 's1')`,
		`INSERT INTO videos (id, slug, corpus) VALUES (4, 'odd-name', 's1')`,
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	rec := footageGET(t, "/api/footage/days")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var days []footageDay
	if err := json.Unmarshal(rec.Body.Bytes(), &days); err != nil {
		t.Fatal(err)
	}
	if want := []footageDay{{"2018-05-14", 1}, {"2026-03-07", 2}}; !reflect.DeepEqual(days, want) {
		t.Errorf("days = %+v, want %+v", days, want)
	}

	rec = footageGET(t, "/api/footage/days/2026-03-07")
	var day footageDayResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &day); err != nil {
		t.Fatal(err)
	}
	if len(day.Clips) != 2 || day.Clips[0].ID != 1 || day.Clips[1].ID != 2 {
		t.Errorf("day clips = %+v, want ids 1 then 2", day.Clips)
	}
	if rec := footageGET(t, "/api/footage/days/2026-03-09"); rec.Code != http.StatusNotFound {
		t.Errorf("empty day: status = %d, want 404", rec.Code)
	}
	if rec := footageGET(t, "/api/footage/days/march"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad day: status = %d, want 400", rec.Code)
	}
}

// A piece is listed when either auto-trim or a reviewer dropped it whole,
// newest footage first, with the clip it aired as when a reviewer kept it.
func TestFootageDropped(t *testing.T) {
	db := testdb.New(t)
	for _, q := range []string{
		`INSERT INTO videos (id, slug, corpus) VALUES (9, '20260502080000_000124_s0180', 's2')`,
		`INSERT INTO video_cuts (slug, proposal, ruling) VALUES
		 ('20260501120000_000123_s0000', '{"verdict":"drop"}', NULL),
		 ('20260501120000_000123_s0180', '{"verdict":"trim"}', NULL),
		 ('20260502080000_000124_s0000', '{"verdict":"keep"}', '{"verdict":"drop"}'),
		 ('20260502080000_000124_s0180', '{"verdict":"drop"}', '{"verdict":"keep"}')`,
	} {
		if err := db.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	rec := footageGET(t, "/api/footage/dropped")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got []droppedCut
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, c := range got {
		slugs = append(slugs, c.Slug)
	}
	want := []string{"20260502080000_000124_s0180", "20260502080000_000124_s0000", "20260501120000_000123_s0000"}
	if !reflect.DeepEqual(slugs, want) {
		t.Fatalf("slugs = %v, want %v", slugs, want)
	}
	if got[0].VideoID != 9 || got[1].VideoID != 0 || !strings.Contains(string(got[1].Ruling), "drop") {
		t.Errorf("got %+v", got)
	}
}
