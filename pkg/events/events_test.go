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
		return Login(ctx, cfg, "someone", uuid.New(), Airing{})
	}},
	{"Logout", "logout", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Logout(ctx, cfg, "someone", uuid.New(), nil, Airing{})
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
	{"StateCrossing", "state_crossing", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return StateCrossing(ctx, cfg, "Utah", "Colorado", 42, true)
	}},
	{"CommandRefused", "command_refused", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return CommandRefused(ctx, cfg, CommandRefusal{
			Username: "someone", Command: "!watchtime", Reason: RefusedUnknown,
		})
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
					sqlmock.AnyArg(), // video_id
					sqlmock.AnyArg(), // video_ts_sec
					sqlmock.AnyArg(), // meta
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

// A session event records the footage the viewer arrived on or left on. This
// pairing is what per-clip churn is computed from, so both columns have to
// reach the row — a login that drops them is indistinguishable from one on an
// idle stream.
func TestSessionWritersRecordAiring(t *testing.T) {
	ts := 12.5
	for _, tc := range []struct {
		name      string
		wantEvent string
		call      func(context.Context, *c.TripbotConfig) error
	}{
		{"Login", "login", func(ctx context.Context, cfg *c.TripbotConfig) error {
			return Login(ctx, cfg, "someone", uuid.New(), Airing{VideoID: 42, TsSec: &ts})
		}},
		{"Logout", "logout", func(ctx context.Context, cfg *c.TripbotConfig) error {
			return Logout(ctx, cfg, "someone", uuid.New(), nil, Airing{VideoID: 42, TsSec: &ts})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := installMockDB(t)
			mock.ExpectQuery(`INSERT INTO "events"`).
				WithArgs(
					"someone",        // username
					"twitch",         // platform
					tc.wantEvent,     // event
					sqlmock.AnyArg(), // session_id
					sqlmock.AnyArg(), // date_created
					nil,              // extra_miles_earned
					42,               // video_id
					12.5,             // video_ts_sec
					nil,              // meta
				).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

			if err := tc.call(context.Background(), &c.TripbotConfig{Platform: "twitch"}); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Error(err)
			}
		})
	}
}

// The zero Airing — an instance with no player, or nothing on screen — writes
// NULL into both columns rather than a 0 that would read as "clip 0, first
// second" to every query downstream.
func TestSessionWritersZeroAiringWritesNull(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone", "twitch", "login",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, // video_id
			nil, // video_ts_sec
			nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := Login(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "someone", uuid.New(), Airing{}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A state_crossing row is a system event: empty username, the new clip's id,
// and a meta document naming the transition. The exact JSON matters — the
// rollups and any Grafana query address it as meta->>'from' / meta->>'to' /
// meta->>'sequential'.
func TestStateCrossingRow(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"",               // username: system event, no actor
			"twitch",         // platform
			"state_crossing", // event
			sqlmock.AnyArg(), // session_id
			sqlmock.AnyArg(), // date_created
			nil,              // extra_miles_earned
			42,               // video_id
			nil,              // video_ts_sec: clip-level writer
			`{"from":"Utah","to":"Colorado","sequential":true}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := StateCrossing(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "Utah", "Colorado", 42, true); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A crossing onto a clip with no DB row (LoadOrCreate failed) still records,
// with a NULL video_id — mirroring viewstats.RecordPlay.
func TestStateCrossingZeroVideoIDWritesNull(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"", "twitch", "state_crossing",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, // video_id
			nil,
			`{"from":"Utah","to":"Colorado","sequential":false}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := StateCrossing(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "Utah", "Colorado", 0, false); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
