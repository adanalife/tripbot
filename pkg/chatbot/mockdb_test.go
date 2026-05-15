package chatbot

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB stands up a sqlmock-backed *gorm.DB and installs it as the
// process-wide singleton via database.SetGormDB. Commands that go through
// package-level helpers like users.Find / scoreboards.TopUsers will route to
// the mock instead of attempting a real postgres connection.
//
// SkipDefaultTransaction stops GORM from wrapping every write in BEGIN/COMMIT,
// which would otherwise force every test to mock those bookends.
//
// Returned mock is used to set query expectations. The singleton is reset on
// test cleanup so subsequent tests start clean.
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
