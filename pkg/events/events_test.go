package events

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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

// writers is every public event writer, invoked against a config. Keeping them
// in one list means a new event kind that forgets the read-only guard or the
// platform stamp shows up here rather than in production data.
var writers = []struct {
	name      string
	wantEvent string
	call      func(context.Context, *c.TripbotConfig) error
}{
	{"Login", "login", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Login(ctx, cfg, "someone", uuid.New())
	}},
	{"Logout", "logout", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Logout(ctx, cfg, "someone", uuid.New(), nil)
	}},
	{"Subscribe", "subscribe", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Subscribe(ctx, cfg, "someone")
	}},
	{"Unsubscribe", "unsubscribe", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Unsubscribe(ctx, cfg, "someone")
	}},
	{"Correction", "correction", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Correction(ctx, cfg, "someone", -1.5)
	}},
}

// A read-only instance must write no events at all. There's no mock DB
// installed here, so any writer that reaches GORM panics or errors on a nil
// singleton rather than silently passing.
func TestWritersRespectReadOnly(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			err := w.call(context.Background(), &c.TripbotConfig{ReadOnly: true, Platform: "twitch"})
			if !errors.Is(err, terrors.ErrReadOnly) {
				t.Fatalf("err = %v, want terrors.ErrReadOnly", err)
			}
		})
	}
}

// Every event row carries the instance's platform and its own event name. An
// unstamped platform would collapse the per-platform rollups onto one bucket,
// and a wrong event name would break the login/logout pairing miles are
// derived from.
func TestWritersStampPlatformAndEvent(t *testing.T) {
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			mock := installMockDB(t)
			mock.ExpectQuery(`INSERT INTO "events"`).
				WithArgs(
					sqlmock.AnyArg(), // username
					"tiktok",         // platform
					w.wantEvent,      // event
					sqlmock.AnyArg(), // session_id
					sqlmock.AnyArg(), // date_created
					sqlmock.AnyArg(), // extra_miles_earned
				).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

			if err := w.call(context.Background(), &c.TripbotConfig{Platform: "tiktok"}); err != nil {
				t.Fatalf("%s: %v", w.name, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}
