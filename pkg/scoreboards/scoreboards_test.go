package scoreboards

import (
	"context"
	"reflect"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// createUser inserts a viewer row and returns its ID. Usernames are unique per
// (platform, username), so tests can reuse a name across platforms.
func createUser(t *testing.T, db *gorm.DB, username, platform string, isBot bool) uint16 {
	t.Helper()
	var id uint16
	row := db.Raw(
		`INSERT INTO users (username, platform, is_bot) VALUES (?, ?, ?) RETURNING id`,
		username, platform, isBot,
	).Row()
	if err := row.Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	return id
}

// createScoreboardOn inserts a scoreboard for an arbitrary platform, which
// createScoreboard() can't do (it always stamps this instance's platform).
func createScoreboardOn(t *testing.T, db *gorm.DB, name, platform string) uint16 {
	t.Helper()
	var id uint16
	row := db.Raw(
		`INSERT INTO scoreboards (name, platform) VALUES (?, ?) RETURNING id`,
		name, platform,
	).Row()
	if err := row.Scan(&id); err != nil {
		t.Fatalf("insert scoreboard %q/%q: %v", name, platform, err)
	}
	return id
}

func insertScore(t *testing.T, db *gorm.DB, userID, scoreboardID uint16, value float32) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO scores (user_id, scoreboard_id, value) VALUES (?, ?, ?)`,
		userID, scoreboardID, value,
	).Error
	if err != nil {
		t.Fatalf("insert score: %v", err)
	}
}

// TestTopUsers covers the whole read contract in one board: descending order,
// float formatting, and the three exclusions (bots, the channel owner, and
// users belonging to another platform).
func TestTopUsers(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	sb, err := findOrCreateScoreboard(ctx, "miles_2026_07")
	if err != nil {
		t.Fatalf("findOrCreateScoreboard: %v", err)
	}

	// The real write path for viewers on this platform.
	createUser(t, db, "alice", c.Conf.Platform, false)
	createUser(t, db, "bob", c.Conf.Platform, false)
	for _, w := range []struct {
		username string
		value    float32
	}{{"alice", 10.5}, {"bob", 42.5}} {
		if err := AddToScoreByName(ctx, w.username, "miles_2026_07", w.value); err != nil {
			t.Fatalf("AddToScoreByName(%s): %v", w.username, err)
		}
	}

	// Rows that must not surface: a bot, the channel owner, and a viewer from
	// another platform whose score hangs off this platform's board.
	botID := createUser(t, db, "tripbot4000", c.Conf.Platform, true)
	ownerID := createUser(t, db, strings.ToLower(c.Conf.ChannelName), c.Conf.Platform, false)
	otherPlatformID := createUser(t, db, "carol", "youtube", false)
	for _, id := range []uint16{botID, ownerID, otherPlatformID} {
		insertScore(t, db, id, sb.ID, 999)
	}

	got := TopUsers(ctx, "miles_2026_07", 10)
	want := [][]string{{"bob", "42.5"}, {"alice", "10.5"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopUsers = %v, want %v", got, want)
	}

	if got := TopUsers(ctx, "miles_2026_07", 1); !reflect.DeepEqual(got, [][]string{{"bob", "42.5"}}) {
		t.Errorf("TopUsers with size=1 = %v, want just the top row", got)
	}
}

// A same-named board on another platform must not leak into this instance's
// leaderboard — scoreboard names are global, uniqueness is (name, platform).
func TestTopUsers_OtherPlatformBoardExcluded(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	otherBoard := createScoreboardOn(t, db, "miles_2026_07", "youtube")
	userID := createUser(t, db, "alice", c.Conf.Platform, false)
	insertScore(t, db, userID, otherBoard, 42.5)

	if got := TopUsers(ctx, "miles_2026_07", 10); len(got) != 0 {
		t.Fatalf("expected no rows from the other platform's board, got %v", got)
	}
}

func TestTopUsers_UnknownScoreboard(t *testing.T) {
	testdb.New(t)
	if got := TopUsers(context.Background(), "no_such_board_2026_07", 10); len(got) != 0 {
		t.Fatalf("expected no rows, got %v", got)
	}
}

func TestFindOrCreateScoreboard_CreatesThenFinds(t *testing.T) {
	testdb.New(t)
	ctx := context.Background()

	created, err := findOrCreateScoreboard(ctx, "miles_2026_07")
	if err != nil {
		t.Fatalf("findOrCreateScoreboard (create): %v", err)
	}
	if created.ID == 0 || created.Name != "miles_2026_07" || created.Platform != c.Conf.Platform {
		t.Fatalf("unexpected scoreboard: %+v", created)
	}
	if created.DateCreated.IsZero() {
		t.Errorf("expected date_created stamped on insert, got %+v", created)
	}

	found, err := findOrCreateScoreboard(ctx, "miles_2026_07")
	if err != nil {
		t.Fatalf("findOrCreateScoreboard (find): %v", err)
	}
	if found.ID != created.ID {
		t.Fatalf("expected the existing row %d, got a second row %d", created.ID, found.ID)
	}
}

// A board named miles_2026_07 already existing on another platform must not be
// adopted: this instance gets its own row, so a youtube bot can never attach
// scores to twitch's same-named board.
func TestFindOrCreateScoreboard_PlatformScoped(t *testing.T) {
	db := testdb.New(t)

	otherID := createScoreboardOn(t, db, "miles_2026_07", "youtube")

	sb, err := findOrCreateScoreboard(context.Background(), "miles_2026_07")
	if err != nil {
		t.Fatalf("findOrCreateScoreboard: %v", err)
	}
	if sb.ID == otherID {
		t.Fatalf("adopted the other platform's board (id %d)", otherID)
	}
	if sb.Platform != c.Conf.Platform {
		t.Fatalf("expected platform %q, got %+v", c.Conf.Platform, sb)
	}
}

// GetScoreByName / AddToScoreByName round-trip: increments accumulate on the
// one (user_id, scoreboard_id) row rather than racing in a second one.
func TestAddToScoreByName_Accumulates(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	userID := createUser(t, db, "alice", c.Conf.Platform, false)

	if got, err := GetScoreByName(ctx, "alice", "miles_2026_07"); err != nil || got != 0 {
		t.Fatalf("GetScoreByName on a fresh board = %v, %v; want 0, nil", got, err)
	}
	for _, v := range []float32{1.5, 2} {
		if err := AddToScoreByName(ctx, "alice", "miles_2026_07", v); err != nil {
			t.Fatalf("AddToScoreByName: %v", err)
		}
	}

	got, err := GetScoreByName(ctx, "alice", "miles_2026_07")
	if err != nil {
		t.Fatalf("GetScoreByName: %v", err)
	}
	if got != 3.5 {
		t.Errorf("score = %v, want 3.5", got)
	}

	var rows int64
	if err := db.Table("scores").Where("user_id = ?", userID).Count(&rows).Error; err != nil {
		t.Fatalf("count scores: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected increments to land on one row, got %d rows", rows)
	}
}
