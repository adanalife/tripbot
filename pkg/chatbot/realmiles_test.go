package chatbot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	"gorm.io/gorm"
)

// seedRealMiles writes a rollup row plus one 1.5-mile clip played 5 minutes
// ago on twitch, so the command sees both a completed-session total and a
// live-session portion.
func seedRealMiles(t *testing.T, db *gorm.DB, username string, rolled float64) {
	t.Helper()
	if err := db.Exec(`INSERT INTO user_rollups (platform, username, real_miles)
	                   VALUES ('twitch', ?, ?)`, username, rolled).Error; err != nil {
		t.Fatalf("insert rollup: %v", err)
	}
	var vidID int64
	err := db.Raw(`INSERT INTO videos (slug, lat, lng, date_filmed, miles_driven)
	               VALUES ('realmiles_cmd_clip', 40.0, -111.0, now(), 1.5) RETURNING id`).Scan(&vidID).Error
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
	err = db.Exec(`INSERT INTO video_plays (platform, video_id, started_at)
	               VALUES ('twitch', ?, ?)`, vidID, time.Now().Add(-5*time.Minute)).Error
	if err != nil {
		t.Fatalf("insert play: %v", err)
	}
}

func TestRealMilesCmd_SumsRollupAndLiveSession(t *testing.T) {
	db := testdb.New(t)
	seedRealMiles(t, db, "roadtripper", 10.5)

	app := newTestApp(video.Video{})
	say := captureSay(t, app)
	user := &users.User{Username: "roadtripper", LoggedIn: time.Now().Add(-10 * time.Minute)}

	app.realMilesCmd(context.Background(), user, nil)

	got := say()
	if !strings.Contains(got, "12.0") {
		t.Errorf("expected 10.5 rolled + 1.5 live = 12.0 in output, got %q", got)
	}
	if !strings.Contains(got, "@roadtripper") {
		t.Errorf("expected @username in output, got %q", got)
	}
}

func TestRealMilesCmd_ExcludesPlaysBeforeLogin(t *testing.T) {
	db := testdb.New(t)
	seedRealMiles(t, db, "latecomer", 10.5)

	app := newTestApp(video.Video{})
	say := captureSay(t, app)
	// Logged in after the seeded play — only the rolled-up total counts.
	user := &users.User{Username: "latecomer", LoggedIn: time.Now()}

	app.realMilesCmd(context.Background(), user, nil)

	if got := say(); !strings.Contains(got, "10.5") {
		t.Errorf("expected only the rolled-up 10.5 in output, got %q", got)
	}
}

func TestRealMilesCmd_ZeroOdometerStillAnswers(t *testing.T) {
	testdb.New(t)

	app := newTestApp(video.Video{})
	say := captureSay(t, app)
	user := &users.User{Username: "brandnew", LoggedIn: time.Now()}

	app.realMilesCmd(context.Background(), user, nil)

	if got := say(); !strings.Contains(got, "odometer") {
		t.Errorf("expected a zero-odometer message, got %q", got)
	}
}

func TestRealMilesIsSubscriberOnly(t *testing.T) {
	for _, cmd := range builtTestApp.commands {
		if cmd.Trigger == "!realmiles" {
			if !cmd.RequiresSubscriber {
				t.Error("!realmiles must be subscriber-only")
			}
			return
		}
	}
	t.Error("!realmiles not found in the command registry")
}
