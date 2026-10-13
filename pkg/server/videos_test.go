package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
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

// One clip's record: its place, its neighbours, and every airing on every
// platform, counted.
func TestVideoHandler(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO videos (id, slug, state, city, city_m, lat, lng, prev_vid, next_vid, date_filmed)
		 VALUES (7, '2018_0514_224801_013', 'Utah', 'Moab', 0, 38.57, -109.55, 6, 8, '2018-05-14 22:48:01+00')`,
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
