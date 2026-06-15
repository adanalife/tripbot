package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// withMigrationSeam stubs the DB-backed schema_migrations read so the handler
// renders without a real database.
func withMigrationSeam(t *testing.T, version int64, dirty bool, err error) {
	t.Helper()
	saved := readSchemaMigration
	t.Cleanup(func() { readSchemaMigration = saved })
	readSchemaMigration = func(context.Context) (int64, bool, error) {
		return version, dirty, err
	}
}

func fetchMigration(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/db/migration", http.HandlerFunc(migrationVersionAPIHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/db/migration", nil))
	return rec
}

func TestMigrationVersionAPIHandler_OK(t *testing.T) {
	withMigrationSeam(t, 20, false, nil)
	rec := fetchMigration(t)

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var got migrationVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if !got.OK || got.Version != 20 || got.Dirty {
		t.Errorf("unexpected migration version: %+v", got)
	}
}

func TestMigrationVersionAPIHandler_Dirty(t *testing.T) {
	withMigrationSeam(t, 21, true, nil)
	rec := fetchMigration(t)
	var got migrationVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK || got.Version != 21 || !got.Dirty {
		t.Errorf("expected dirty version 21, got %+v", got)
	}
}

// TestMigrationVersionAPIHandler_DBError covers an unreadable schema_migrations
// row (DB unreachable, missing table): OK=false so the console renders
// "unknown" instead of a misleading version 0.
func TestMigrationVersionAPIHandler_DBError(t *testing.T) {
	withMigrationSeam(t, 0, false, errors.New("connection refused"))
	rec := fetchMigration(t)
	var got migrationVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OK {
		t.Errorf("expected OK=false on read error, got %+v", got)
	}
	// snake_case wire format the console reads.
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("expected snake_case keys: %s", rec.Body.String())
	}
}
