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

func newFindTestApp(t *testing.T, search Search) (*App, *recordingVLC, *recordingChat) {
	t.Helper()
	app := newTestApp(video.Video{})
	recVLC := &recordingVLC{}
	recChat := &recordingChat{}
	app.VLC = recVLC
	app.Chat = recChat
	app.Search = search
	return app, recVLC, recChat
}

func TestFindCmd_JumpsToClosestHit(t *testing.T) {
	skipIfDarwin(t)
	search := &recordingSearch{Hits: []SearchHit{
		{Slug: "2018_0514_224801_013", TsSec: 163.5, State: "Nevada", Distance: 0.21},
		{Slug: "2018_0601_000000_001", TsSec: 12, State: "Utah", Distance: 0.55},
	}}
	app, recVLC, recChat := newFindTestApp(t, search)

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"snowy", "mountains"})
	})

	if len(search.Queries) != 1 || search.Queries[0] != "snowy mountains" {
		t.Fatalf("expected one search for %q, got %v", "snowy mountains", search.Queries)
	}
	want := `PlayFileAtTimestamp("2018_0514_224801_013.MP4", 163.5)`
	if len(recVLC.Calls) != 1 || recVLC.Calls[0] != want {
		t.Errorf("expected VLC call %q, got %v", want, recVLC.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "Jumping") || !strings.Contains(got, "Nevada") {
		t.Errorf("expected a jump message naming the state, got %q", got)
	}
}

func TestFindCmd_NoHits(t *testing.T) {
	skipIfDarwin(t)
	app, recVLC, recChat := newFindTestApp(t, &recordingSearch{Hits: nil})

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"unicorns"})
	})

	if len(recVLC.Calls) != 0 {
		t.Errorf("expected no VLC jump on a miss, got %v", recVLC.Calls)
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
	app, recVLC, recChat := newFindTestApp(t, search)

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"something"})
	})

	if len(recVLC.Calls) != 0 {
		t.Errorf("expected no VLC jump for a too-distant hit, got %v", recVLC.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "Couldn't find") {
		t.Errorf("expected a not-found reply, got %q", got)
	}
}

func TestFindCmd_SearchError(t *testing.T) {
	skipIfDarwin(t)
	app, recVLC, recChat := newFindTestApp(t, &recordingSearch{Err: errors.New("responder down")})

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), []string{"anything"})
	})

	if len(recVLC.Calls) != 0 {
		t.Errorf("expected no VLC jump on search error, got %v", recVLC.Calls)
	}
	if got := recChat.Output(); !strings.Contains(got, "isn't available") {
		t.Errorf("expected an unavailable reply, got %q", got)
	}
}

func TestFindCmd_NoArgsShowsUsage(t *testing.T) {
	skipIfDarwin(t)
	search := &recordingSearch{}
	app, recVLC, recChat := newFindTestApp(t, search)

	runAsAdmin(t, func() {
		app.findCmd(context.Background(), newTestUser(adminUser), nil)
	})

	if len(search.Queries) != 0 {
		t.Errorf("expected no search with empty args, got %v", search.Queries)
	}
	if len(recVLC.Calls) != 0 {
		t.Errorf("expected no VLC jump with empty args, got %v", recVLC.Calls)
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

func TestVectorLiteral(t *testing.T) {
	got := vectorLiteral([]float32{0.1, 0.2, 0.5})
	if got != "[0.1,0.2,0.5]" {
		t.Errorf("vectorLiteral = %q, want [0.1,0.2,0.5]", got)
	}
	if got := vectorLiteral(nil); got != "[]" {
		t.Errorf("vectorLiteral(nil) = %q, want []", got)
	}
}
