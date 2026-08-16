package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/gorilla/mux"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installServerMockDB swaps the process-wide GORM handle for a sqlmock-backed
// one, mirroring pkg/events's installMockDB, so the round-trip test can assert
// on the row the handler writes without a live postgres.
func installServerMockDB(t *testing.T) sqlmock.Sqlmock {
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

func consoleActionRouter(s *Server) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/api/events/console-action", http.HandlerFunc(s.consoleActionHandler)).Methods("POST")
	return r
}

func postConsoleAction(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/events/console-action", strings.NewReader(body))
	consoleActionRouter(s).ServeHTTP(rec, req)
	return rec
}

// A valid report writes one console_action row — system event, meta carrying
// action/target/detail — and answers a bodyless 204.
func TestConsoleActionHandler_RecordsAndAnswers204(t *testing.T) {
	mock := installServerMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"",               // username: system event, no actor
			sqlmock.AnyArg(), // platform (testConf carries none)
			"console_action", // event
			sqlmock.AnyArg(), // session_id
			sqlmock.AnyArg(), // date_created
			nil,              // extra_miles_earned
			nil,              // video_id
			nil,              // video_ts_sec
			`{"action":"scale","target":"obs-tiktok","detail":"replicas 0→1"}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	rec := postConsoleAction(t, New(testConf),
		`{"action":"scale","target":"obs-tiktok","detail":"replicas 0→1"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204\n%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Detail is optional; action and target are not.
func TestConsoleActionHandler_DetailOptional(t *testing.T) {
	mock := installServerMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"", sqlmock.AnyArg(), "console_action",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, nil,
			`{"action":"obs_refresh","target":"obs-twitch"}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	rec := postConsoleAction(t, New(testConf), `{"action":"obs_refresh","target":"obs-twitch"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204\n%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Bad payloads are rejected before any DB work — no mock DB is installed
// here, so a handler that reached the writer would fail on the nil singleton
// rather than silently passing.
func TestConsoleActionHandler_BadPayloadIs400(t *testing.T) {
	long := strings.Repeat("x", 600)
	cases := map[string]string{
		"not json":       `{"action": nope`,
		"missing action": `{"target":"obs-tiktok"}`,
		"missing target": `{"action":"scale"}`,
		"blank action":   `{"action":"   ","target":"obs-tiktok"}`,
		"long action":    `{"action":"` + long[:200] + `","target":"obs-tiktok"}`,
		"long target":    `{"action":"scale","target":"` + long[:200] + `"}`,
		"long detail":    `{"action":"scale","target":"obs-tiktok","detail":"` + long + `"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := postConsoleAction(t, New(testConf), body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400\n%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// A read-only instance drops the row (events.record's guard) but still
// answers 204: the console's report is fire-and-forget, and read-only is this
// instance's policy, not the console's error.
func TestConsoleActionHandler_ReadOnlyDropsSilently(t *testing.T) {
	s := New(&c.TripbotConfig{Environment: "testing", ReadOnly: true})
	rec := postConsoleAction(t, s, `{"action":"scale","target":"obs-tiktok"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204\n%s", rec.Code, rec.Body.String())
	}
}
