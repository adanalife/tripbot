package scoreboards

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB mirrors pkg/chatbot's helper: a sqlmock-backed *gorm.DB
// installed as the process-wide singleton.
func installMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	database.SetGormDB(gdb)
	t.Cleanup(func() {
		database.SetGormDB(nil)
		_ = sqlDB.Close()
	})
	return mock
}

// TestTopUsers_PlatformScoping pins the per-platform contract with strict
// args: the read must scope both the scoreboard row and the joined users to
// this instance's platform (loose regexes have silently absorbed missing
// platform filters before).
func TestTopUsers_PlatformScoping(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores" JOIN scoreboards ON scores\.scoreboard_id = scoreboards\.id JOIN users ON scores\.user_id = users\.id WHERE \(scoreboards\.name = \$1 AND scoreboards\.platform = \$2\) AND \(users\.is_bot = false AND users\.platform = \$3 AND users\.username != \$4\) ORDER BY scores\.value DESC LIMIT \$5`).
		WithArgs("miles_2026_07", c.Conf.Platform, c.Conf.Platform, strings.ToLower(c.Conf.ChannelName), 3).
		WillReturnRows(sqlmock.NewRows([]string{"username", "value"}).
			AddRow("alice", float32(42.5)))

	got := TopUsers(context.Background(), "miles_2026_07", 3)

	if len(got) != 1 || got[0][0] != "alice" || got[0][1] != "42.5" {
		t.Fatalf("unexpected leaderboard: %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// findOrCreateScoreboard must look up and stamp the instance's platform so a
// youtube instance can never attach scores to twitch's same-named board.
func TestFindOrCreateScoreboard_PlatformScoped(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE "scoreboards"\."name" = \$1 AND "scoreboards"\."platform" = \$2 ORDER BY "scoreboards"\."id" LIMIT \$3`).
		WithArgs("miles_2026_07", c.Conf.Platform, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "platform"}).
			AddRow(7, "miles_2026_07", c.Conf.Platform))

	sb, err := findOrCreateScoreboard(context.Background(), "miles_2026_07")
	if err != nil {
		t.Fatalf("findOrCreateScoreboard: %v", err)
	}
	if sb.ID != 7 || sb.Platform != c.Conf.Platform {
		t.Fatalf("unexpected scoreboard: %+v", sb)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
