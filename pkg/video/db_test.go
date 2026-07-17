package video

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// link writes from.next_vid = to.ID through the production helper, so the
// chains these tests walk are built the same way the corpus importer builds
// the real one.
func link(t *testing.T, from, to Video) {
	t.Helper()
	if err := from.SetNextVid(context.Background(), to); err != nil {
		t.Fatalf("SetNextVid(%d -> %d): %v", from.ID, to.ID, err)
	}
}

// reload re-reads a video by ID. SetNextVid updates the row, not the caller's
// struct, so a walk must start from the persisted state.
func reload(t *testing.T, id int) Video {
	t.Helper()
	vid, err := loadById(context.Background(), int64(id))
	if err != nil {
		t.Fatalf("loadById(%d): %v", id, err)
	}
	return vid
}

func TestVideoNext_ReturnsFirstUnflagged(t *testing.T) {
	db := testdb.New(t)

	start := insertVideo(t, db, Video{Slug: "2018_0514_224801_001"})
	flagged := insertVideo(t, db, Video{Slug: "2018_0514_224801_002", Flagged: true})
	want := insertVideo(t, db, Video{Slug: "2018_0514_224801_003", State: "Oregon", Lat: 45.5, Lng: -122.6})

	// start -> flagged (skipped) -> want (returned)
	link(t, start, flagged)
	link(t, flagged, want)

	got, err := reload(t, start.ID).Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v, want nil", err)
	}
	if got.ID != want.ID {
		t.Errorf("Next() = id %d (slug %q), want id %d (slug %q)", got.ID, got.Slug, want.ID, want.Slug)
	}
	if got.Flagged {
		t.Error("Next() returned a flagged video")
	}
	// The whole row round-trips, so a drifted column tag fails here.
	if got.Slug != want.Slug || got.State != "Oregon" || got.Lat != 45.5 || got.Lng != -122.6 {
		t.Errorf("Next() row = %+v, want the persisted %+v", got, want)
	}
}

func TestVideoNext_BrokenChainReturnsError(t *testing.T) {
	db := testdb.New(t)

	start := insertVideo(t, db, Video{Slug: "2018_0514_224801_010"})
	// Dangle next_vid off the end of the table: videos.next_vid carries no FK,
	// so a real corpus can (and does) hold links to rows that aren't there.
	missingID := start.ID + 10_000
	if err := db.Exec(`UPDATE videos SET next_vid = ? WHERE id = ?`, missingID, start.ID).Error; err != nil {
		t.Fatalf("dangle next_vid: %v", err)
	}

	if _, err := reload(t, start.ID).Next(context.Background()); err == nil {
		t.Fatal("Next() over a dangling next_vid returned nil error, want error")
	}
}

func TestVideoNext_AllFlaggedCycleReturnsError(t *testing.T) {
	db := testdb.New(t)

	// Two flagged videos pointing at each other: a walk that isn't bounded by
	// the playlist length spins here forever.
	a := insertVideo(t, db, Video{Slug: "2018_0514_224801_020", Flagged: true})
	b := insertVideo(t, db, Video{Slug: "2018_0514_224801_021", Flagged: true})
	link(t, a, b)
	link(t, b, a)

	if _, err := reload(t, a.ID).Next(context.Background()); err == nil {
		t.Fatal("Next() over an all-flagged cycle returned nil error, want error")
	}
}

func TestLoadOrCreate_CreatesThenLoadsSameRow(t *testing.T) {
	testdb.New(t)
	ctx := context.Background()

	created, err := LoadOrCreate(ctx, "/footage/2018_0514_224801_030.MP4")
	if err != nil {
		t.Fatalf("LoadOrCreate (create path): %v", err)
	}
	if created.ID == 0 {
		t.Fatal("LoadOrCreate returned an unsaved video (ID 0)")
	}
	if created.Slug != "2018_0514_224801_030" {
		t.Errorf("Slug = %q, want %q", created.Slug, "2018_0514_224801_030")
	}
	// A runtime-created clip has no GPS fix, so save() flags it and records the
	// coords as missing (see db.go save()).
	if !created.Flagged || created.CoordSource != CoordSourceMissing {
		t.Errorf("created video = flagged %v coord_source %q, want flagged true / %q",
			created.Flagged, created.CoordSource, CoordSourceMissing)
	}
	if created.DateCreated.IsZero() {
		t.Error("expected date_created stamped on insert")
	}
	if !created.DateFilmed.Equal(created.toDate()) {
		t.Errorf("date_filmed = %v, want the slug's timestamp %v", created.DateFilmed, created.toDate())
	}

	// Second call hits the load path — same row, no duplicate insert.
	loaded, err := LoadOrCreate(ctx, "/footage/2018_0514_224801_030.MP4")
	if err != nil {
		t.Fatalf("LoadOrCreate (load path): %v", err)
	}
	if loaded.ID != created.ID {
		t.Errorf("second LoadOrCreate made a new row %d, want the existing %d", loaded.ID, created.ID)
	}
}

func TestFindRandomByState(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	want := insertVideo(t, db, Video{Slug: "2018_0514_224801_040", State: "Oregon", Lat: 45.5, Lng: -122.6})
	insertVideo(t, db, Video{Slug: "2018_0514_224801_041", State: "Nevada"})

	// Abbrev and long form both resolve to the stored title-cased state.
	for _, state := range []string{"OR", "oregon"} {
		got, err := FindRandomByState(ctx, state)
		if err != nil {
			t.Fatalf("FindRandomByState(%q): %v", state, err)
		}
		if got.ID != want.ID {
			t.Errorf("FindRandomByState(%q) = id %d (state %q), want id %d", state, got.ID, got.State, want.ID)
		}
	}
}

func TestFindNextDaytime(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	// Denver (UTC-6 in May); date_filmed drives the walk. Use far-future dates
	// so no seed/2018 rows fall after the current clip and skew the query.
	// backdate sets an exact UTC instant per clip.
	lat, lng := 39.7392, -104.9903
	backdate := func(v Video, iso string) {
		if err := db.Exec(`UPDATE videos SET date_filmed = ?::timestamptz WHERE id = ?`, iso, v.ID).Error; err != nil {
			t.Fatalf("backdate %d: %v", v.ID, err)
		}
	}

	// current: 22:00 MDT May 14 (night). next-day night, then the first daytime
	// clip of the following morning (noon), then a later daytime clip.
	current := insertVideo(t, db, Video{Slug: "2099_0514_220000_001", Lat: lat, Lng: lng})
	nextNight := insertVideo(t, db, Video{Slug: "2099_0515_030000_002", Lat: lat, Lng: lng})
	lateDay := insertVideo(t, db, Video{Slug: "2099_0515_150000_004", Lat: lat, Lng: lng})
	morning := insertVideo(t, db, Video{Slug: "2099_0515_120000_003", Lat: lat, Lng: lng})

	backdate(current, "2099-05-15T04:00:00Z")   // 22:00 MDT May 14 — night, day May 14
	backdate(nextNight, "2099-05-15T09:00:00Z") // 03:00 MDT May 15 — night, day May 15
	backdate(morning, "2099-05-15T18:00:00Z")   // 12:00 MDT May 15 — daytime, day May 15
	backdate(lateDay, "2099-05-15T21:00:00Z")   // 15:00 MDT May 15 — daytime, day May 15

	got, err := FindNextDaytime(ctx, reload(t, current.ID))
	if err != nil {
		t.Fatalf("FindNextDaytime: %v", err)
	}
	// Skips the current day and the next day's pre-dawn night clip, landing on
	// the following morning's first daytime clip — not the later afternoon one.
	if got.ID != morning.ID {
		t.Errorf("FindNextDaytime = id %d (slug %q), want the next morning id %d (slug %q)",
			got.ID, got.Slug, morning.ID, morning.Slug)
	}
}

func TestFindNextDaytime_NoDaytimeAhead(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	lat, lng := 39.7392, -104.9903
	backdate := func(v Video, iso string) {
		if err := db.Exec(`UPDATE videos SET date_filmed = ?::timestamptz WHERE id = ?`, iso, v.ID).Error; err != nil {
			t.Fatalf("backdate %d: %v", v.ID, err)
		}
	}

	current := insertVideo(t, db, Video{Slug: "2099_0614_220000_001", Lat: lat, Lng: lng})
	onlyNight := insertVideo(t, db, Video{Slug: "2099_0615_030000_002", Lat: lat, Lng: lng})
	backdate(current, "2099-06-15T04:00:00Z")   // 22:00 MDT June 14 — night
	backdate(onlyNight, "2099-06-15T09:00:00Z") // 03:00 MDT June 15 — night

	_, err := FindNextDaytime(ctx, reload(t, current.ID))
	if _, ok := err.(*terrors.NoDaytimeFoundError); !ok {
		t.Fatalf("FindNextDaytime err = %v, want *NoDaytimeFoundError", err)
	}
}

func TestCorpusRoute_OrdersByFilmTimeAndExcludesFlaggedAndZeroes(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	// CorpusRoute reads the whole table, so give this test's rows coordinates
	// nothing else would produce and assert only on those.
	var (
		earlyCoord = [2]float64{41.11, -71.11}
		lateCoord  = [2]float64{42.22, -72.22}
		flagCoord  = [2]float64{43.33, -73.33}
	)
	later := insertVideo(t, db, Video{Slug: "2018_0514_224801_051", Lat: lateCoord[0], Lng: lateCoord[1]})
	earlier := insertVideo(t, db, Video{Slug: "2018_0514_224801_050", Lat: earlyCoord[0], Lng: earlyCoord[1]})
	insertVideo(t, db, Video{Slug: "2018_0514_224801_052", Lat: flagCoord[0], Lng: flagCoord[1], Flagged: true})
	insertVideo(t, db, Video{Slug: "2018_0514_224801_053"}) // 0/0: excluded

	// date_filmed drives the ordering; insert order deliberately doesn't match.
	backdate := func(v Video, iso string) {
		if err := db.Exec(`UPDATE videos SET date_filmed = ?::timestamptz WHERE id = ?`, iso, v.ID).Error; err != nil {
			t.Fatalf("backdate %d: %v", v.ID, err)
		}
	}
	backdate(earlier, "2018-05-14T00:00:00Z")
	backdate(later, "2018-05-15T00:00:00Z")

	indexOf := func(route [][2]float64, want [2]float64) int {
		for i, c := range route {
			if c == want {
				return i
			}
		}
		return -1
	}

	route := CorpusRoute(ctx)
	early, late := indexOf(route, earlyCoord), indexOf(route, lateCoord)
	if early < 0 || late < 0 {
		t.Fatalf("CorpusRoute() omitted an unflagged clip: early=%d late=%d in %v", early, late, route)
	}
	if early > late {
		t.Errorf("CorpusRoute() not ordered by date_filmed: earlier clip at %d, later at %d", early, late)
	}
	if i := indexOf(route, flagCoord); i >= 0 {
		t.Errorf("CorpusRoute() included a flagged clip at %d", i)
	}
	if i := indexOf(route, [2]float64{0, 0}); i >= 0 {
		t.Errorf("CorpusRoute() included a 0/0 clip at %d", i)
	}
}
