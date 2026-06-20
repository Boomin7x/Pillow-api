//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/kodiahbertrand/pillow/test/testhelpers"
)

func TestIntegration_MigrationsRoundTrip(t *testing.T) {
	ctx := context.Background()
	dsn, cleanup := testhelpers.NewPostgresDSN(t, ctx)
	defer cleanup()

	m, err := migrate.New("file://"+testhelpers.MigrationsDir(), dsn)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if err := m.Down(); err != nil {
		t.Fatalf("migrate down (rollback): %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("migrate up after rollback: %v", err)
	}
}
