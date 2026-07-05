package rollups

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB mirrors pkg/chatbot's helper: a sqlmock-backed *gorm.DB
// installed as the process-wide singleton so Reconcile routes to the mock.
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

// expectSnapshots registers the two idempotent month-end snapshot inserts that
// open every tick. Strict args pin the board-name parameter in all three
// positions (insert value, board lookup, once-only guard).
func expectSnapshots(mock sqlmock.Sqlmock) {
	for _, board := range []string{scoreboards.PreviousMilesScoreboard(), scoreboards.PreviousGuessScoreboard()} {
		mock.ExpectExec(`INSERT INTO scoreboard_snapshots`).
			WithArgs(board, board, board).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
}

func TestReconcile_NoNewEventsIsANoOp(t *testing.T) {
	mock := installMockDB(t)

	mock.ExpectBegin()
	expectSnapshots(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_event_id FROM rollup_watermarks WHERE name = $1 FOR UPDATE`)).
		WithArgs(watermarkName).
		WillReturnRows(sqlmock.NewRows([]string{"last_event_id"}).AddRow(100))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(id), 0) FROM events`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(100))
	mock.ExpectCommit()

	Reconcile(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestReconcile_RecomputesDirtyUsersAndAdvancesWatermark(t *testing.T) {
	mock := installMockDB(t)

	mock.ExpectBegin()
	expectSnapshots(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT last_event_id FROM rollup_watermarks WHERE name = $1 FOR UPDATE`)).
		WithArgs(watermarkName).
		WillReturnRows(sqlmock.NewRows([]string{"last_event_id"}).AddRow(100))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(id), 0) FROM events`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(150))
	// Strict args pin the watermark window: recompute exactly (100, 150].
	mock.ExpectExec(`INSERT INTO user_rollups`).
		WithArgs(int64(100), int64(150)).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE rollup_watermarks SET last_event_id = $1, date_updated = now() WHERE name = $2`)).
		WithArgs(int64(150), watermarkName).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	Reconcile(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestReconcile_SnapshotErrorRollsBack(t *testing.T) {
	mock := installMockDB(t)

	mock.ExpectBegin()
	board := scoreboards.PreviousMilesScoreboard()
	mock.ExpectExec(`INSERT INTO scoreboard_snapshots`).
		WithArgs(board, board, board).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()

	Reconcile(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestReconcile_ReadOnlySkipsEverything(t *testing.T) {
	mock := installMockDB(t)

	orig := c.Conf.ReadOnly
	c.Conf.ReadOnly = true
	t.Cleanup(func() { c.Conf.ReadOnly = orig })

	Reconcile(context.Background())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("ReadOnly Reconcile touched the DB: %v", err)
	}
}
