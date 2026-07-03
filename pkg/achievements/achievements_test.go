package achievements

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/video"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB mirrors pkg/rollups' helper: a sqlmock-backed *gorm.DB
// installed as the process-wide singleton so HandleVideoChange routes to it.
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

func expectPlayInsert(mock sqlmock.Sqlmock, videoID int) {
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO video_plays (video_id) VALUES ($1)`)).
		WithArgs(videoID).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestHandleVideoChange_ZeroIDIsANoOp(t *testing.T) {
	mock := installMockDB(t)
	if msgs := HandleVideoChange(context.Background(), video.Video{}, []string{"alice"}); msgs != nil {
		t.Errorf("expected no messages, got %v", msgs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("zero-ID video touched the DB: %v", err)
	}
}

func TestHandleVideoChange_ReadOnlySkipsEverything(t *testing.T) {
	mock := installMockDB(t)
	orig := c.Conf.ReadOnly
	c.Conf.ReadOnly = true
	t.Cleanup(func() { c.Conf.ReadOnly = orig })

	HandleVideoChange(context.Background(), video.Video{ID: 7}, []string{"alice"})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("ReadOnly HandleVideoChange touched the DB: %v", err)
	}
}

func TestHandleVideoChange_AwardsStateVisit(t *testing.T) {
	mock := installMockDB(t)
	v := video.Video{ID: 42, State: "California", Flagged: true} // Flagged skips the landmark pass

	mock.ExpectBegin()
	expectPlayInsert(mock, 42)
	for _, viewer := range []string{"alice", "bob"} {
		mock.ExpectExec(`INSERT INTO user_state_days`).
			WithArgs("twitch", viewer, "California").
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	// alice crosses the first-visit tier; nobody hits 10 or 100.
	mock.ExpectQuery(`INSERT INTO achievements`).
		WithArgs("state-california-1", "First visit to California", "twitch", "California", 1).
		WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("alice"))
	mock.ExpectQuery(`INSERT INTO achievements`).
		WithArgs("state-california-10", "10th visit to California", "twitch", "California", 10).
		WillReturnRows(sqlmock.NewRows([]string{"username"}))
	mock.ExpectQuery(`INSERT INTO achievements`).
		WithArgs("state-california-100", "100th visit to California", "twitch", "California", 100).
		WillReturnRows(sqlmock.NewRows([]string{"username"}))
	mock.ExpectCommit()

	msgs := HandleVideoChange(context.Background(), v, []string{"alice", "bob"})

	want := "🏆 Achievement unlocked — First visit to California: @alice"
	if len(msgs) != 1 || msgs[0] != want {
		t.Errorf("messages = %v, want [%q]", msgs, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestHandleVideoChange_AwardsLandmark(t *testing.T) {
	mock := installMockDB(t)
	// On the Golden Gate Bridge, no state (isolates the landmark pass).
	v := video.Video{ID: 9, Lat: 37.8199, Lng: -122.4786}

	mock.ExpectBegin()
	expectPlayInsert(mock, 9)
	mock.ExpectExec(`INSERT INTO achievements`).
		WithArgs("twitch", "alice", "landmark-golden-gate-bridge", "Saw the Golden Gate Bridge").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	msgs := HandleVideoChange(context.Background(), v, []string{"alice"})

	want := "🏆 Achievement unlocked — Saw the Golden Gate Bridge: @alice"
	if len(msgs) != 1 || msgs[0] != want {
		t.Errorf("messages = %v, want [%q]", msgs, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestDistanceKm(t *testing.T) {
	// SF ↔ LA is ~559 km great-circle.
	if d := distanceKm(37.7749, -122.4194, 34.0522, -118.2437); d < 540 || d > 580 {
		t.Errorf("SF-LA distance = %.0f km, want ~559", d)
	}
	// A clip a few hundred meters from Old Faithful is inside its radius.
	if d := distanceKm(44.4605, -110.8281, 44.4630, -110.8300); d > landmarks[0].RadiusKm {
		t.Errorf("near-Old-Faithful distance = %.2f km, want within %v", d, landmarks[0].RadiusKm)
	}
}
