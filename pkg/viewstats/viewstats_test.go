package viewstats

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB stands up a sqlmock-backed *gorm.DB as the process-wide
// singleton. Mirrors the pattern from pkg/chatbot/mockdb_test.go.
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

// notZeroTime matches any non-zero timestamp arg — the guard that
// autoCreateTime actually stamped the column instead of writing 0001-01-01
// over its DEFAULT (the pkg/events regression).
type notZeroTime struct{}

func (notZeroTime) Match(v driver.Value) bool {
	tm, ok := v.(time.Time)
	return ok && !tm.IsZero()
}

func TestRecordPlayAndSample(t *testing.T) {
	mock := installMockDB(t)
	ctx := context.Background()

	// A sample before any play carries a NULL video_id.
	mock.ExpectQuery(`INSERT INTO "viewer_samples" .*RETURNING "id"`).
		WithArgs(sqlmock.AnyArg(), 3, nil, notZeroTime{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	RecordSample(ctx, 3)

	mock.ExpectQuery(`INSERT INTO "video_plays" .*RETURNING "id"`).
		WithArgs(sqlmock.AnyArg(), 42, "Utah", false, 38.5, -109.5, notZeroTime{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	RecordPlay(ctx, 42, "Utah", false, 38.5, -109.5)

	// After the play, samples are tagged with its video id.
	mock.ExpectQuery(`INSERT INTO "viewer_samples" .*RETURNING "id"`).
		WithArgs(sqlmock.AnyArg(), 5, 42, notZeroTime{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	RecordSample(ctx, 5)

	// A play with no DB row (videoID 0) writes NULL and resets the sample tag.
	mock.ExpectQuery(`INSERT INTO "video_plays" .*RETURNING "id"`).
		WithArgs(sqlmock.AnyArg(), nil, "", true, 0.0, 0.0, notZeroTime{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(2))
	RecordPlay(ctx, 0, "", true, 0, 0)

	mock.ExpectQuery(`INSERT INTO "viewer_samples" .*RETURNING "id"`).
		WithArgs(sqlmock.AnyArg(), 5, nil, notZeroTime{}).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	RecordSample(ctx, 5)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}
