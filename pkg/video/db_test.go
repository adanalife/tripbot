package video

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
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
