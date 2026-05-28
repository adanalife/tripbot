package feature

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// newMockDB stands up a sqlmock-backed *gorm.DB suitable for unit-testing
// the Postgres-backed flag client without a real database.
func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gdb, mock
}

// expectFlags queues a successful SELECT on feature_flags returning the given rows.
func expectFlags(mock sqlmock.Sqlmock, rows ...flagRow) {
	r := sqlmock.NewRows([]string{
		"key", "description", "enabled",
		"enabled_for_usernames", "enabled_for_roles",
		"target_removal_date", "created_at", "updated_at",
	})
	for _, row := range rows {
		r.AddRow(
			row.Key, row.Description, row.Enabled,
			pqArrayLiteral(row.EnabledForUsernames),
			pqArrayLiteral(row.EnabledForRoles),
			row.TargetRemovalDate, time.Now(), time.Now(),
		)
	}
	mock.ExpectQuery(`SELECT \* FROM "feature_flags"`).WillReturnRows(r)
}

// pqArrayLiteral renders a []string the way the postgres driver returns a
// TEXT[] column over the wire — `{a,b,c}` form — so pq.StringArray can
// unmarshal it in tests.
func pqArrayLiteral(s []string) string {
	if len(s) == 0 {
		return "{}"
	}
	out := "{"
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	out += "}"
	return out
}

func TestPostgresClient_InitialLoad(t *testing.T) {
	db, mock := newMockDB(t)
	expectFlags(mock, flagRow{
		Key:                 "chatbot.ascii",
		Description:         "experimental ascii command",
		Enabled:             false,
		EnabledForUsernames: []string{"dana"},
		EnabledForRoles:     []string{"mod"},
		TargetRemovalDate:   time.Now().Add(30 * 24 * time.Hour),
	})

	c, err := NewPostgresClient(context.Background(), db, time.Minute)
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}

	if !c.Bool(context.Background(), "chatbot.ascii", EvalContext{Username: "dana"}) {
		t.Error("dana should be in the username allowlist")
	}
	if !c.Bool(context.Background(), "chatbot.ascii", EvalContext{Roles: []string{"mod"}}) {
		t.Error("mod role should match the role allowlist")
	}
	if c.Bool(context.Background(), "chatbot.ascii", EvalContext{Roles: []string{"regular"}}) {
		t.Error("regular user should not be enabled")
	}
	if c.Bool(context.Background(), "chatbot.unknown", EvalContext{}) {
		t.Error("unknown key should evaluate to false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestPostgresClient_InitialLoadError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery(`SELECT \* FROM "feature_flags"`).
		WillReturnError(errors.New("connection refused"))

	if _, err := NewPostgresClient(context.Background(), db, time.Minute); err == nil {
		t.Error("expected initial load to surface the DB error")
	}
}

func TestPostgresClient_RefreshFailureRetainsCache(t *testing.T) {
	db, mock := newMockDB(t)
	// Initial load: one flag, globally enabled.
	expectFlags(mock, flagRow{
		Key:               "chatbot.report_to_discord",
		Description:       "discord webhook for !report",
		Enabled:           true,
		TargetRemovalDate: time.Now().Add(30 * 24 * time.Hour),
	})
	c, err := NewPostgresClient(context.Background(), db, time.Minute)
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}
	if !c.Bool(context.Background(), "chatbot.report_to_discord", EvalContext{}) {
		t.Fatal("initial load: flag should be enabled")
	}

	// Manual refresh that fails — cache should be retained.
	mock.ExpectQuery(`SELECT \* FROM "feature_flags"`).
		WillReturnError(errors.New("connection refused"))
	if err := c.refresh(context.Background()); err == nil {
		t.Error("expected refresh to surface error")
	}
	if !c.Bool(context.Background(), "chatbot.report_to_discord", EvalContext{}) {
		t.Error("flag should still evaluate to enabled after a failed refresh")
	}

	// Recovery refresh: flag is now globally disabled.
	expectFlags(mock, flagRow{
		Key:               "chatbot.report_to_discord",
		Description:       "discord webhook for !report",
		Enabled:           false,
		TargetRemovalDate: time.Now().Add(30 * 24 * time.Hour),
	})
	if err := c.refresh(context.Background()); err != nil {
		t.Fatalf("recovery refresh: %v", err)
	}
	if c.Bool(context.Background(), "chatbot.report_to_discord", EvalContext{}) {
		t.Error("flag should be disabled after successful refresh")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestPostgresClient_EmptyTable(t *testing.T) {
	db, mock := newMockDB(t)
	expectFlags(mock) // zero rows
	c, err := NewPostgresClient(context.Background(), db, time.Minute)
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}
	if c.Bool(context.Background(), "any.key", EvalContext{}) {
		t.Error("empty table should evaluate every key to false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
