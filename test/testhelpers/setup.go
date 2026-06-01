package testhelpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// NewPostgres returns a real *gorm.DB connected to a test Postgres instance.
// In CI, use testcontainers-go to spin up the container before calling this.
// The DSN is expected via the TEST_DATABASE_URL environment variable or the
// provided dsn argument.
func NewPostgres(t *testing.T, ctx context.Context, dsn string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("testhelpers: open postgres: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("testhelpers: get sql.DB: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := sqlDB.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("testhelpers: postgres not ready: %v", err)
	}

	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("testhelpers: close db: %v", err)
		}
	})

	return db
}

// MustExec runs a raw SQL statement against db and fails the test on error.
func MustExec(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(fmt.Sprintf(sql, args...)).Error; err != nil {
		t.Fatalf("testhelpers: exec %q: %v", sql, err)
	}
}
