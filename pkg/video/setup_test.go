package video

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
	"gorm.io/gorm"
)

// insertVideo writes a videos row and returns it with the SERIAL-assigned ID
// populated, so callers can link chains by real ID.
func insertVideo(t *testing.T, db *gorm.DB, vid Video) Video {
	t.Helper()
	if err := db.Create(&vid).Error; err != nil {
		t.Fatalf("insert video %q: %v", vid.Slug, err)
	}
	return vid
}

// playCount returns how many video_plays rows point at the given video, so
// tests can assert on the play the Player actually persisted rather than on a
// scripted INSERT. Scoped by video_id — the table is shared.
func playCount(t *testing.T, db *gorm.DB, videoID int) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM video_plays WHERE video_id = ?`, videoID).Scan(&n).Error; err != nil {
		t.Fatalf("count video_plays for video %d: %v", videoID, err)
	}
	return n
}

// recordingOnscreens is an interface fake satisfying the Player's onscreens
// dependency, capturing each GPS overlay call by method name.
type recordingOnscreens struct {
	calls []string
}

func (r *recordingOnscreens) ShowGPSImage(_ context.Context, _ time.Duration) error {
	r.calls = append(r.calls, "ShowGPSImage")
	return nil
}

func (r *recordingOnscreens) HideGPSImage(_ context.Context) error {
	r.calls = append(r.calls, "HideGPSImage")
	return nil
}

// fakeVLCServer stands up an httptest.Server that responds to /vlc/current
// with the value pointed to by current. Tests mutate *current to change what
// the next Player.GetCurrentlyPlaying call observes.
//
// Returns a *vlc-client.Client configured to talk to the fake.
func fakeVLCServer(t *testing.T, current *string) *vlcClient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/vlc/current" {
			_, _ = w.Write([]byte(*current))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	// vlcClient.New builds the URL as "http://" + host, so strip the scheme
	// from the httptest URL before handing it over. nil publisher disables the
	// NATS mirror — this rig exercises the HTTP path only.
	return vlcClient.New(strings.TrimPrefix(srv.URL, "http://"), nil, "test", "twitch")
}
