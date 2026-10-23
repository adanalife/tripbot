package video

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

// A viewer holds one mark of a kind per clip: marking again moves it, while
// another viewer's mark and the console's stay put.
func TestSetMarkMovesAViewersOwnMark(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	vid := insertVideo(t, db, Video{Slug: "2018_0514_224801_013", State: "Utah"})
	for _, m := range []Mark{
		{VideoID: vid.ID, Kind: "trim", Sec: 10, By: "ada", Platform: "twitch"},
		{VideoID: vid.ID, Kind: "trim", Sec: 20, By: "ada", Platform: "twitch"},
		{VideoID: vid.ID, Kind: "trim", Sec: 20, By: "ada", Platform: "youtube"},
		{VideoID: vid.ID, Kind: "star", Sec: 20, By: "ada", Platform: "twitch"},
		{VideoID: vid.ID, Kind: "trim", Sec: 30, By: "bo", Platform: "twitch", Note: "fog"},
		{VideoID: vid.ID, Kind: "star", Sec: 40},
		{VideoID: vid.ID, Kind: "star", Sec: 41},
	} {
		if err := SetMark(ctx, m); err != nil {
			t.Fatalf("SetMark(%+v): %v", m, err)
		}
	}
	var got []string
	db.Raw(`SELECT kind || ' ' || source_ts_sec::int || ' ' || username || ' ' || platform || ' ' || note
FROM video_moments WHERE video_id = ? ORDER BY source_ts_sec, kind, platform`, vid.ID).Scan(&got)
	want := []string{
		"star 20 ada twitch ", "trim 20 ada twitch ", "trim 20 ada youtube ",
		"trim 30 bo twitch fog", "star 40   ", "star 41   ",
	}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}
