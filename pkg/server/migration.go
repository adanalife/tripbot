package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/adanalife/tripbot/pkg/database"
)

// migrationVersion is the wire format the standalone tripbot-console reads via
// GET /api/db/migration. The console holds no DB access of its own, so it
// proxies here over the in-namespace Service to show which golang-migrate
// migration the env's Postgres is on. version/dirty come straight from the
// schema_migrations table golang-migrate maintains; ok is false when the row
// couldn't be read (e.g. DB unreachable), so the console can render a graceful
// "unknown" instead of a misleading 0.
type migrationVersion struct {
	OK      bool  `json:"ok"`
	Version int64 `json:"version"`
	Dirty   bool  `json:"dirty"`
}

// readSchemaMigration is the data seam the migration handler reads through,
// overridable in tests so the handler renders without a real DB. golang-migrate
// keeps a single row in schema_migrations holding the current version + a dirty
// flag (set true when a migration failed partway).
var readSchemaMigration = func(ctx context.Context) (version int64, dirty bool, err error) {
	row := database.GormDB().WithContext(ctx).
		Raw("SELECT version, dirty FROM schema_migrations LIMIT 1").Row()
	if err := row.Err(); err != nil {
		return 0, false, err
	}
	if err := row.Scan(&version, &dirty); err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}

// gatherMigrationVersion reads the current schema-migration version through the
// DB seam. A read error (DB unreachable, missing table) returns OK=false so the
// console can show "unknown" rather than a misleading version.
func gatherMigrationVersion(ctx context.Context) migrationVersion {
	version, dirty, err := readSchemaMigration(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "couldn't read schema migration version", "err", err)
		return migrationVersion{OK: false}
	}
	return migrationVersion{OK: true, Version: version, Dirty: dirty}
}

// migrationVersionAPIHandler serves GET /api/db/migration: the current
// golang-migrate schema version as JSON, for the standalone tripbot-console to
// surface which migration the env's Postgres is on (the console holds no DB
// access — it links over to here).
func migrationVersionAPIHandler(w http.ResponseWriter, r *http.Request) {
	mv := gatherMigrationVersion(r.Context())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(mv); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode migration version", "err", err)
	}
}
