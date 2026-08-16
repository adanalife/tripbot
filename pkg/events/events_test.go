package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
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
	{"Follow", "follow", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Follow(ctx, cfg, "someone")
	}},
	{"Correction", "correction", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Correction(ctx, cfg, "someone", -1.5)
	}},
	{"WatchdogRestart", "watchdog_restart", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return WatchdogRestart(ctx, cfg, "tiktok", WatchdogOutcomeOK)
	}},
	{"WatchdogRecovered", "watchdog_recovered", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return WatchdogRecovered(ctx, cfg, "tiktok")
	}},
	{"StateCrossing", "state_crossing", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return StateCrossing(ctx, cfg, "Utah", "Colorado", 42, true)
	}},
	{"GuessSubmitted", "guess_submitted", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return GuessSubmitted(ctx, cfg, GuessSubmission{
			Username: "someone", Guessed: "Wyoming", Actual: "Colorado",
		})
	}},
	{"Timewarp", "timewarp", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return Timewarp(ctx, cfg, Warp{
			Username: "someone", Source: WarpSourceCommand,
		})
	}},
	{"CommandRefused", "command_refused", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return CommandRefused(ctx, cfg, CommandRefusal{
			Username: "someone", Command: "!watchtime", Reason: RefusedUnknown,
		})
	}},
	{"CommandRan", "command_run", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return CommandRan(ctx, cfg, CommandRun{
			Username: "someone", Command: "!location",
		})
	}},
	{"ConsoleAction", "console_action", func(ctx context.Context, cfg *c.TripbotConfig) error {
		return ConsoleAction(ctx, cfg, "scale", "obs-tiktok", "replicas 0→1")
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

// A guess_submitted row pairs the normalized guess with the state actually on
// screen, right or wrong, stamped with the clip being guessed at. The exact
// JSON matters — rollups address it as meta->>'guessed' / meta->>'actual' /
// meta->>'correct' / meta->>'distance_mi'. A correct guess is 0 miles off by
// definition, and the 0 stays in the meta so "measured perfect" never reads
// like "couldn't measure".
func TestGuessSubmittedRow(t *testing.T) {
	ts := 12.5
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone",         // username
			"twitch",          // platform
			"guess_submitted", // event
			sqlmock.AnyArg(),  // session_id
			sqlmock.AnyArg(),  // date_created
			nil,               // extra_miles_earned
			42,                // video_id
			12.5,              // video_ts_sec
			`{"guessed":"Colorado","actual":"Colorado","correct":true,"distance_mi":0}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := GuessSubmitted(context.Background(), &c.TripbotConfig{Platform: "twitch"}, GuessSubmission{
		Username: "someone", Guessed: "Colorado", Actual: "Colorado", Correct: true,
		VideoID: 42, TsSec: &ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A miss records correct:false explicitly — the key never drops out of the
// meta, or a rollup computing accuracy would read absence as anything it
// likes — and carries the miss distance in centroid-to-centroid miles. Zero
// airing context writes NULL, not clip 0.
func TestGuessSubmittedMissAndZeroAiringWritesNull(t *testing.T) {
	// The expected distance comes from the same pure helpers the writer uses;
	// this pins the meta's key name, placement, and centroid pairing rather
	// than the haversine arithmetic (helpers' own tests cover that).
	glat, glng, _ := helpers.StateCentroid("Wyoming")
	alat, alng, _ := helpers.StateCentroid("Colorado")
	dist, err := json.Marshal(helpers.MilesBetween(glat, glng, alat, alng))
	if err != nil {
		t.Fatal(err)
	}

	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone", "twitch", "guess_submitted",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, // video_id
			nil, // video_ts_sec
			`{"guessed":"Wyoming","actual":"Colorado","correct":false,"distance_mi":`+string(dist)+`}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := GuessSubmitted(context.Background(), &c.TripbotConfig{Platform: "twitch"}, GuessSubmission{
		Username: "someone", Guessed: "Wyoming", Actual: "Colorado",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A guessed territory has no centroid to measure from, so the distance key is
// omitted rather than written as a fake value — a wrong guess that can't be
// measured must stay distinguishable from both a perfect one and a far one.
func TestGuessSubmittedUnmeasurableGuessOmitsDistance(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone", "twitch", "guess_submitted",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, // video_id
			nil, // video_ts_sec
			`{"guessed":"Puerto Rico","actual":"Colorado","correct":false}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := GuessSubmitted(context.Background(), &c.TripbotConfig{Platform: "twitch"}, GuessSubmission{
		Username: "someone", Guessed: "Puerto Rico", Actual: "Colorado",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A timewarp row carries the departing clip in video_id and the landing clip
// in the meta, so "warped away from X onto Y" is one row. Rollups address the
// meta as meta->>'source' / meta->>'to'.
func TestTimewarpRow(t *testing.T) {
	ts := 47.5
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone",                    // username
			"twitch",                     // platform
			"timewarp",                   // event
			sqlmock.AnyArg(),             // session_id
			sqlmock.AnyArg(),             // date_created
			nil,                          // extra_miles_earned
			42,                           // video_id: the clip the warp left
			47.5,                         // video_ts_sec
			`{"source":"guess","to":88}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := Timewarp(context.Background(), &c.TripbotConfig{Platform: "twitch"}, Warp{
		Username: "someone", Source: WarpSourceGuess,
		VideoID: 42, TsSec: &ts, ToVideoID: 88,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A warp with no known clips (an instance whose player has no rows staged)
// still records the trigger: NULL video_id, and no bogus to:0 in the meta.
func TestTimewarpZeroClipsWriteNull(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone", "twitch", "timewarp",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil,                    // video_id
			nil,                    // video_ts_sec
			`{"source":"command"}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := Timewarp(context.Background(), &c.TripbotConfig{Platform: "twitch"}, Warp{
		Username: "someone", Source: WarpSourceCommand,
	})
	if err != nil {
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

// A command_run row pairs the canonical trigger with what the viewer actually
// typed and the args they passed, stamped with the airing clip and playhead.
// The exact JSON matters — rollups address it as meta->>'command' /
// meta->>'typed' / meta->>'args', and no reason key ever appears: a run has
// nothing to explain, so runs and refusals stay distinguishable by kind alone.
func TestCommandRunRow(t *testing.T) {
	ts := 33.25
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone",        // username
			"twitch",         // platform
			"command_run",    // event
			sqlmock.AnyArg(), // session_id
			sqlmock.AnyArg(), // date_created
			nil,              // extra_miles_earned
			42,               // video_id
			33.25,            // video_ts_sec
			`{"command":"!guess","typed":"!florida","args":"florida"}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := CommandRan(context.Background(), &c.TripbotConfig{Platform: "twitch"}, CommandRun{
		Username: "someone", Command: "!guess", Typed: "!florida", Args: "florida",
		VideoID: 42, TsSec: &ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A run with no airing context writes NULL rather than a 0 that would read as
// "clip 0, first second", and empty typed/args drop out of the meta rather
// than writing empty keys.
func TestCommandRunZeroAiringWritesNull(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"someone", "twitch", "command_run",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil,                     // video_id
			nil,                     // video_ts_sec
			`{"command":"!uptime"}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err := CommandRan(context.Background(), &c.TripbotConfig{Platform: "twitch"}, CommandRun{
		Username: "someone", Command: "!uptime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
