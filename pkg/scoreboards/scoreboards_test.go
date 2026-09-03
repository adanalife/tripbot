package scoreboards

import (
	"context"
	"reflect"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// createUser inserts a viewer row and returns its ID. Usernames are unique per
// (platform, username), so tests can reuse a name across platforms.
// testConf is the config the scoreboard queries under test are scoped to.
var testConf = &c.TripbotConfig{Environment: "testing", Platform: "twitch", ChannelName: "test"}

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

// createOptedOutUser inserts a viewer who is not a bot but has asked to stay
// off the leaderboards, and returns its ID.
func createOptedOutUser(t *testing.T, db *gorm.DB, username, platform string) uint16 {
	t.Helper()
	var id uint16
	row := db.Raw(
		`INSERT INTO users (username, platform, exclude_from_leaderboard) VALUES (?, ?, TRUE) RETURNING id`,
		username, platform,
	).Row()
	if err := row.Scan(&id); err != nil {
		t.Fatalf("insert opted-out user %q: %v", username, err)
	}
	return id
}

// createScoreboardOn inserts a scoreboard for an arbitrary platform, which the
// package's own lookups can't do (they always scope to this instance's
// platform).
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
// float formatting, and the four exclusions (bots, opted-out accounts, the
// channel owner, and users belonging to another platform).
func TestTopUsers(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	sb := createScoreboardOn(t, db, "miles_2026_07", testConf.Platform)

	// The real write path for viewers on this platform.
	createUser(t, db, "alice", testConf.Platform, false)
	createUser(t, db, "bob", testConf.Platform, false)
	for _, w := range []struct {
		username string
		value    float32
	}{{"alice", 10.5}, {"bob", 42.5}} {
		if err := AddToScoreByName(ctx, testConf.Platform, w.username, "miles_2026_07", w.value); err != nil {
			t.Fatalf("AddToScoreByName(%s): %v", w.username, err)
		}
	}

	// Rows that must not surface: a bot, an opted-out human, the channel owner,
	// and a viewer from another platform whose score hangs off this platform's
	// board.
	botID := createUser(t, db, "tripbot4000", testConf.Platform, true)
	optedOutID := createOptedOutUser(t, db, "optedout", testConf.Platform)
	ownerID := createUser(t, db, testConf.ChannelName, testConf.Platform, false)
	otherPlatformID := createUser(t, db, "carol", "youtube", false)
	for _, id := range []uint16{botID, optedOutID, ownerID, otherPlatformID} {
		insertScore(t, db, id, sb, 999)
	}

	got := TopUsers(ctx, testConf, "miles_2026_07", 10)
	want := [][]string{{"bob", "42.5"}, {"alice", "10.5"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TopUsers = %v, want %v", got, want)
	}

	if got := TopUsers(ctx, testConf, "miles_2026_07", 1); !reflect.DeepEqual(got, [][]string{{"bob", "42.5"}}) {
		t.Errorf("TopUsers with size=1 = %v, want just the top row", got)
	}
}

// Tied scores come back in a fixed order. Without the username tiebreaker
// Postgres may return equal values in any order, and the overlay re-renders
// the board every rotation tick — so tied viewers would visibly swap places
// on a live broadcast. Inserted worst-name-first so a query that only sorted
// by value would have to reorder them to pass.
func TestTopUsers_TiesOrderByUsername(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	sb := createScoreboardOn(t, db, "miles_2026_07", testConf.Platform)
	for _, name := range []string{"carol", "bob", "alice"} {
		insertScore(t, db, createUser(t, db, name, testConf.Platform, false), sb, 12.5)
	}

	want := [][]string{{"alice", "12.5"}, {"bob", "12.5"}, {"carol", "12.5"}}
	for i := 0; i < 3; i++ {
		if got := TopUsers(ctx, testConf, "miles_2026_07", 10); !reflect.DeepEqual(got, want) {
			t.Fatalf("TopUsers call %d = %v, want %v", i+1, got, want)
		}
	}
}

// A same-named board on another platform must not leak into this instance's
// leaderboard — scoreboard names are global, uniqueness is (name, platform).
func TestTopUsers_OtherPlatformBoardExcluded(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	otherBoard := createScoreboardOn(t, db, "miles_2026_07", "youtube")
	userID := createUser(t, db, "alice", testConf.Platform, false)
	insertScore(t, db, userID, otherBoard, 42.5)

	if got := TopUsers(ctx, testConf, "miles_2026_07", 10); len(got) != 0 {
		t.Fatalf("expected no rows from the other platform's board, got %v", got)
	}
}

func TestTopUsers_UnknownScoreboard(t *testing.T) {
	testdb.New(t)
	if got := TopUsers(context.Background(), testConf, "no_such_board_2026_07", 10); len(got) != 0 {
		t.Fatalf("expected no rows, got %v", got)
	}
}

// The add path creates the board it scores on, once, and only for this
// instance's platform: a same-named board on another platform must not be
// adopted, or a youtube bot could attach scores to twitch's board.
func TestAddToScoreByName_CreatesBoardPerPlatform(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	otherID := createScoreboardOn(t, db, "miles_2026_07", "youtube")
	createUser(t, db, "alice", testConf.Platform, false)

	for i := 0; i < 2; i++ {
		if err := AddToScoreByName(ctx, testConf.Platform, "alice", "miles_2026_07", 1); err != nil {
			t.Fatalf("AddToScoreByName: %v", err)
		}
	}

	var boards []struct {
		ID          uint16
		DateCreated time.Time
	}
	if err := db.Raw(
		`SELECT id, date_created FROM scoreboards WHERE name = ? AND platform = ?`,
		"miles_2026_07", testConf.Platform,
	).Scan(&boards).Error; err != nil {
		t.Fatalf("select scoreboards: %v", err)
	}
	if len(boards) != 1 {
		t.Fatalf("expected exactly one board for this platform, got %d", len(boards))
	}
	if boards[0].ID == otherID {
		t.Fatalf("adopted the other platform's board (id %d)", otherID)
	}
	if boards[0].DateCreated.IsZero() {
		t.Errorf("expected date_created stamped on insert, got %v", boards[0].DateCreated)
	}
}

// An unknown username scores nothing rather than landing on someone else's
// row, and reads back as 0 rather than erroring.
func TestScoreByName_UnknownUser(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	knownID := createUser(t, db, "alice", testConf.Platform, false)
	if err := AddToScoreByName(ctx, testConf.Platform, "alice", "miles_2026_07", 5); err != nil {
		t.Fatalf("AddToScoreByName: %v", err)
	}

	if err := AddToScoreByName(ctx, testConf.Platform, "ghost", "miles_2026_07", 99); err != nil {
		t.Fatalf("AddToScoreByName(ghost): %v", err)
	}

	got, err := GetScoreByName(ctx, testConf.Platform, "ghost", "miles_2026_07")
	if err != nil || got != 0 {
		t.Errorf("GetScoreByName(ghost) = %v, %v; want 0, nil", got, err)
	}

	var value float32
	if err := db.Raw(`SELECT value FROM scores WHERE user_id = ?`, knownID).Scan(&value).Error; err != nil {
		t.Fatalf("select score: %v", err)
	}
	if value != 5 {
		t.Errorf("the known user's score = %v, want 5 — the ghost add leaked onto it", value)
	}
}

// GetScoreByName / AddToScoreByName round-trip: increments accumulate on the
// one (user_id, scoreboard_id) row rather than racing in a second one.
func TestAddToScoreByName_Accumulates(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	userID := createUser(t, db, "alice", testConf.Platform, false)

	if got, err := GetScoreByName(ctx, testConf.Platform, "alice", "miles_2026_07"); err != nil || got != 0 {
		t.Fatalf("GetScoreByName on a fresh board = %v, %v; want 0, nil", got, err)
	}
	for _, v := range []float32{1.5, 2} {
		if err := AddToScoreByName(ctx, testConf.Platform, "alice", "miles_2026_07", v); err != nil {
			t.Fatalf("AddToScoreByName: %v", err)
		}
	}

	got, err := GetScoreByName(ctx, testConf.Platform, "alice", "miles_2026_07")
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

// insertSnapshot writes one frozen-board row the way the rollup tick does.
func insertSnapshot(t *testing.T, db *gorm.DB, board, platform string, rank int, username string, value float32) {
	t.Helper()
	err := db.Exec(
		`INSERT INTO scoreboard_snapshots (scoreboard_name, platform, rank, username, value) VALUES (?, ?, ?, ?, ?)`,
		board, platform, rank, username, value,
	).Error
	if err != nil {
		t.Fatalf("insert snapshot row: %v", err)
	}
}

// TestSnapshotTopUsers covers the frozen-board read: rank order rather than
// insertion order, the channel owner dropped, another platform's rows for the
// same board invisible, and the size cap.
func TestSnapshotTopUsers(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	const board = "miles_2026_08"

	insertSnapshot(t, db, board, testConf.Platform, 2, "alice", 10.5)
	insertSnapshot(t, db, board, testConf.Platform, 1, testConf.ChannelName, 999)
	insertSnapshot(t, db, board, testConf.Platform, 3, "bob", 42.5)
	insertSnapshot(t, db, board, "youtube", 1, "carol", 77)

	got := SnapshotTopUsers(ctx, testConf, board, 10)
	want := [][]string{{"alice", "10.5"}, {"bob", "42.5"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SnapshotTopUsers = %v, want %v", got, want)
	}
	if got := SnapshotTopUsers(ctx, testConf, board, 1); !reflect.DeepEqual(got, [][]string{{"alice", "10.5"}}) {
		t.Errorf("SnapshotTopUsers with size=1 = %v, want just the top row", got)
	}
	if got := SnapshotTopUsers(ctx, testConf, "miles_2026_07", 10); len(got) != 0 {
		t.Errorf("SnapshotTopUsers for an unfrozen month = %v, want nothing", got)
	}
}
