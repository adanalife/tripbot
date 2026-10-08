package chatbot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

func TestMissLine(t *testing.T) {
	tests := []struct {
		km, bearing float64
		want        string
	}{
		{23.4, 315, "23 km NW"},
		{613.6, 135, "614 km SE"},
		{1, 0, "1 km N"},
		{0.006, 200, "bullseye"},
	}
	for _, tc := range tests {
		if got := missLine(tc.km, tc.bearing); got != tc.want {
			t.Errorf("missLine(%v, %v) = %q, want %q", tc.km, tc.bearing, got, tc.want)
		}
	}
}

// The game's rows run a daily board end to end: the board, then one /guesses
// read per row. Rank 1 gets its latest closed round; rank 2's newest round is
// still open (nulls), so it falls back to the round before; rank 3's plays name
// someone else; rank 4 errors. Only the first two change.
func TestFetchLeaderboard_GuessrDaily_AddsMissLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/leaderboard":
			fmt.Fprint(w, `{"board":"daily","period":"2026-10-07","rows":[["Patient Delta",14166],["Sam (2)",8489],["Carol",4082],["Dave",10]]}`)
		case "/guesses":
			switch r.URL.Query().Get("rank") {
			case "1":
				// Pin due north-west of the truth, after a round due south.
				fmt.Fprint(w, `{"name":"Patient Delta","rows":[
					{"km":50,"guess_lat":39,"guess_lng":-100,"answer_lat":40,"answer_lng":-100},
					{"km":23.4,"guess_lat":40.15,"guess_lng":-100.2,"answer_lat":40,"answer_lng":-100}]}`)
			case "2":
				fmt.Fprint(w, `{"name":"Sam","rows":[
					{"km":0.2,"guess_lat":40,"guess_lng":-100,"answer_lat":40,"answer_lng":-100},
					{"km":9,"guess_lat":null,"guess_lng":null,"answer_lat":null,"answer_lng":null}]}`)
			case "3":
				fmt.Fprint(w, `{"name":"Mallory","rows":[{"km":5,"guess_lat":1,"guess_lng":1,"answer_lat":0,"answer_lng":0}]}`)
			default:
				http.Error(w, "boom", http.StatusInternalServerError)
			}
		}
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	_, rows := app.fetchLeaderboard(context.Background(), guessrDailyLeaderboard)

	want := [][]string{
		{"Patient Delta · 23 km NW", "14166"},
		{"Sam (2) · bullseye", "8489"},
		{"Carol", "4082"},
		{"Dave", "10"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
}

// The miss line is optional: a game that serves the board but not the plays
// leaves the board exactly as it was.
func TestWithMisses_FetchFails_BoardUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	board := [][]string{{"Patient Delta", "14166"}, {"Carol", "4082"}}
	got := withMisses(context.Background(), "monthly", board)
	if !reflect.DeepEqual(got, [][]string{{"Patient Delta", "14166"}, {"Carol", "4082"}}) {
		t.Errorf("board changed on a failed fetch: %q", got)
	}
	srv.Close() // unreachable, not just a 404
	if got := withMisses(context.Background(), "monthly", board); !reflect.DeepEqual(got, board) {
		t.Errorf("board changed with the game down: %q", got)
	}
}
