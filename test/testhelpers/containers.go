//go:build integration

package testhelpers

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
)

func NewPostgresContainer(t *testing.T, ctx context.Context) (*gorm.DB, func()) {
	t.Helper()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("pillow_test"),
		tcpostgres.WithUsername("pillow"),
		tcpostgres.WithPassword("secret"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("testhelpers: start postgres container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("testhelpers: postgres connection string: %v", err)
	}

	runMigrations(t, dsn)

	db := NewPostgres(t, ctx, dsn)

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("testhelpers: terminate postgres container: %v", err)
		}
	}
	return db, cleanup
}

func NewRedisContainer(t *testing.T, ctx context.Context) (*redis.Client, func()) {
	t.Helper()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("testhelpers: start redis container: %v", err)
	}

	endpoint, err := container.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("testhelpers: redis endpoint: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: endpoint})
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("testhelpers: redis ping: %v", err)
	}

	cleanup := func() {
		_ = client.Close()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("testhelpers: terminate redis container: %v", err)
		}
	}
	return client, cleanup
}

func runMigrations(t *testing.T, dsn string) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testhelpers: cannot resolve migrations path")
	}
	migrationsPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")

	m, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		t.Fatalf("testhelpers: migrate init: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("testhelpers: migrate up: %v", err)
	}
}
