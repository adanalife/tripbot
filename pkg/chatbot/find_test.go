package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/video"
)

// noopSearch satisfies Search for tests that don't exercise !find — it returns
// no hits.
type noopSearch struct{}

func (noopSearch) Find(_ context.Context, _ string) ([]SearchHit, error) { return nil, nil }

// recordingSearch returns canned hits / error and records the queries it saw,
// so command tests can assert !find behavior without NATS or a model.
type recordingSearch struct {
	Hits    []SearchHit
	Err     error
	Queries []string
}

func (r *recordingSearch) Find(_ context.Context, query string) ([]SearchHit, error) {
	r.Queries = append(r.Queries, query)
	return r.Hits, r.Err
}

func newFindTestApp(t *testing.T, search Search) (*App, *recordingPlayout, *recordingChat) {
	t.Helper()
	app := newTestApp(video.Video{})
	recPlayout := &recordingPlayout{}
	recChat := &recordingChat{}
	app.Playout = recPlayout
	app.Chat = recChat
	app.Search = search
	// !find is feature-flagged; enable it for the behavior tests.
	app.Flags = &recordingFlags{Set: map[string]bool{findFlagKey: true}}
	return app, recPlayout, recChat
}

func TestFindCmd_JumpsToClosestHit(t *testing.T) {
	skipIfDarwin(t)
	pinFindRandIntn(t, 0) // force the closest moment so the jump is assertable
	search := &recordingSearch{Hits: []SearchHit{
		{Slug: "2018_0514_224801_013", TsSec: 163.5, State: "Nevada", Distance: 0.21},
		{Slug: "2018_0601_000000_001", TsSec: 12, State: "Utah", Distance: 0.55},
	}}
	app, recPlayout, recChat := newFindTestApp(t, search)

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"snowy", "mountains"})
	})

	if len(search.Queries) != 1 || search.Queries[0] != "snowy mountains" {
		t.Fatalf("expected one search for %q, got %v", "snowy mountains", search.Queries)
	}
	// Lands findJumpLeadInSec ahead of the matched frame (163.5 - 12 = 151.5).
	want := `PlayFileAtTimestamp("2018_0514_224801_013.MP4", 151.5)`
	if len(recPlayout.Calls) != 1 || recPlayout.Calls[0] != want {
		t.Errorf("expected Playout call %q, got %v", want, recPlayout.Calls)
	}
	// The jump message must not name the state, so a viewer can still guess it.
	if got := recChat.Output(); !strings.Contains(got, "Jumping") || strings.Contains(got, "Nevada") {
		t.Errorf("expected a jump message that omits the state, got %q", got)
	}
}

func TestFindCmd_FlagOff_StaysSilent(t *testing.T) {
	// The flag gate runs before everything (incl. the Darwin guard), so a
	// disabled !find is silent on any OS — no search, no jump, no reply.
	search := &recordingSearch{Hits: []SearchHit{{Slug: "x", TsSec: 1, Distance: 0.1}}}
	app, recPlayout, recChat := newFindTestApp(t, search)
	app.Flags = noopFlags{} // every key false — fresh-deploy state

	app.findCmd(context.Background(), newTestUser(adminUser), []string{"anything"})

	if len(search.Queries) != 0 {
		t.Errorf("expected no search when flag is off, got %v", search.Queries)
	}
	if len(recPlayout.Calls) != 0 {
		t.Errorf("expected no Playout jump when flag is off, got %v", recPlayout.Calls)
	}
	if got := recChat.Output(); got != "" {
		t.Errorf("expected silence when flag is off, got %q", got)
	}
}

func TestFindCmd_NoHits(t *testing.T) {
	skipIfDarwin(t)
	app, recPlayout, recChat := newFindTestApp(t, &recordingSearch{Hits: nil})

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"unicorns"})
	})

	if len(recPlayout.Calls) != 0 {
		t.Errorf("expected no Playout jump on a miss, got %v", recPlayout.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "Couldn't find") {
		t.Errorf("expected a not-found reply, got %q", got)
	}
}

func TestFindCmd_DistanceTooFarIsAMiss(t *testing.T) {
	skipIfDarwin(t)
	// A hit exists but it's past the distance ceiling — treat as a miss rather
	// than yanking the stream to a bad match.
	search := &recordingSearch{Hits: []SearchHit{{Slug: "x", TsSec: 1, Distance: findMaxDistance + 0.01}}}
	app, recPlayout, recChat := newFindTestApp(t, search)

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"something"})
	})

	if len(recPlayout.Calls) != 0 {
		t.Errorf("expected no Playout jump for a too-distant hit, got %v", recPlayout.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "Couldn't find") {
		t.Errorf("expected a not-found reply, got %q", got)
	}
}

func TestFindCmd_SearchError(t *testing.T) {
	skipIfDarwin(t)
	app, recPlayout, recChat := newFindTestApp(t, &recordingSearch{Err: errors.New("responder down")})

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"anything"})
	})

	if len(recPlayout.Calls) != 0 {
		t.Errorf("expected no Playout jump on search error, got %v", recPlayout.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "isn't available") {
		t.Errorf("expected an unavailable reply, got %q", got)
	}
}

func TestFindCmd_NoArgsShowsUsage(t *testing.T) {
	skipIfDarwin(t)
	search := &recordingSearch{}
	app, recPlayout, recChat := newFindTestApp(t, search)

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), nil)
	})

	if len(search.Queries) != 0 {
		t.Errorf("expected no search with empty args, got %v", search.Queries)
	}
	if len(recPlayout.Calls) != 0 {
		t.Errorf("expected no Playout jump with empty args, got %v", recPlayout.Calls)
	}
	if got := recChat.Output(); !strings.Contains(strings.ToLower(got), "usage") {
		t.Errorf("expected a usage reply, got %q", got)
	}
}

// --- searchFrameEmbeddings (pgvector query) ---

func TestSearchFrameEmbeddings_ScansRows(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery("frame_embeddings").
		WillReturnRows(mock.NewRows([]string{"slug", "ts_sec", "state", "distance"}).
			AddRow("2018_0514_224801_013", 163.5, "Nevada", 0.21).
			AddRow("2018_0601_000000_001", 12.0, "Utah", 0.55))

	hits, err := searchFrameEmbeddings(context.Background(), database.GormDB(),
		[]float32{0.1, 0.2, 0.3}, "model-x", nil, nil, 5)
	if err != nil {
		t.Fatalf("searchFrameEmbeddings: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Slug != "2018_0514_224801_013" || hits[0].TsSec != 163.5 || hits[0].State != "Nevada" {
		t.Errorf("unexpected first hit: %+v", hits[0])
	}
	if hits[0].Distance != 0.21 {
		t.Errorf("distance = %v, want 0.21", hits[0].Distance)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestSearchFrameEmbeddings_WithStateAndMonthFilters(t *testing.T) {
	mock := installMockDB(t)
	// The filtered query still hits frame_embeddings; we just confirm it builds
	// and scans cleanly with the optional clauses appended.
	mock.ExpectQuery("frame_embeddings").
		WillReturnRows(mock.NewRows([]string{"slug", "ts_sec", "state", "distance"}).
			AddRow("2018_0514_224801_013", 5.0, "Nevada", 0.3))

	hits, err := searchFrameEmbeddings(context.Background(), database.GormDB(),
		[]float32{0.1, 0.2}, "model-x", []string{"Nevada"}, []int{5}, 5)
	if err != nil {
		t.Fatalf("searchFrameEmbeddings: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestSearchFrameEmbeddings_EmptyVector(t *testing.T) {
	if _, err := searchFrameEmbeddings(context.Background(), nil, nil, "model-x", nil, nil, 5); err == nil {
		t.Error("expected an error for an empty query vector")
	}
}

// pinFindRandIntn forces pickFindHit's random choice to a fixed index for the
// duration of a test, then restores the real source.
func pinFindRandIntn(t *testing.T, idx int) {
	t.Helper()
	orig := findRandIntn
	findRandIntn = func(int) int { return idx }
	t.Cleanup(func() { findRandIntn = orig })
}

func TestPickFindHit_CollapsesAdjacentFramesIntoOneMoment(t *testing.T) {
	// Three near-identical adjacent frames of one clip plus one distinct moment:
	// dedup should leave exactly two moments to randomize over.
	hits := []SearchHit{
		{Slug: "clipA", TsSec: 100.0, Distance: 0.20},
		{Slug: "clipA", TsSec: 101.5, Distance: 0.21},
		{Slug: "clipA", TsSec: 103.0, Distance: 0.22},
		{Slug: "clipB", TsSec: 5.0, Distance: 0.30},
	}
	pinFindRandIntn(t, 1) // second distinct moment
	if got := pickFindHit(hits); got.Slug != "clipB" {
		t.Errorf("index 1 should be the second distinct moment clipB, got %+v", got)
	}
	pinFindRandIntn(t, 0) // first distinct moment = closest frame of clipA
	if got := pickFindHit(hits); got.Slug != "clipA" || got.TsSec != 100.0 {
		t.Errorf("index 0 should be clipA@100 (closest), got %+v", got)
	}
}

func TestPickFindHit_SkipsHitsOverCeiling(t *testing.T) {
	// The nearer hit is over the ceiling; only clipB qualifies. Pinning index 0
	// must return clipB — if the over-ceiling hit were included it'd sort first.
	hits := []SearchHit{
		{Slug: "clipA", TsSec: 1, Distance: findMaxDistance + 0.01},
		{Slug: "clipB", TsSec: 1, Distance: 0.20},
	}
	pinFindRandIntn(t, 0)
	if got := pickFindHit(hits); got.Slug != "clipB" {
		t.Errorf("expected the over-ceiling clipA to be skipped, got %+v", got)
	}
}

func TestVectorLiteral(t *testing.T) {
	got := vectorLiteral([]float32{0.1, 0.2, 0.5})
	if got != "[0.1,0.2,0.5]" {
		t.Errorf("vectorLiteral = %q, want [0.1,0.2,0.5]", got)
	}
	if got := vectorLiteral(nil); got != "[]" {
		t.Errorf("vectorLiteral(nil) = %q, want []", got)
	}
}
