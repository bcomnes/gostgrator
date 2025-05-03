// gostgrator_test.go
package gostgrator_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bcomnes/gostgrator"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

var pgTestConfig gostgrator.Config

// TestMain sets up a temporary Postgres test database and drops it after tests.
func TestMain(m *testing.M) {
	// Connect to default "postgres" database.
	connStr := "host=localhost port=5432 user=postgres dbname=postgres sslmode=disable"
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatalf("failed to ping postgres: %v", err)
	}

	// Drop if exists and then create our test database.
	_, _ = db.Exec("DROP DATABASE IF EXISTS gostgrator_test")
	_, err = db.Exec("CREATE DATABASE gostgrator_test")
	if err != nil {
		log.Fatalf("failed to create test database: %v", err)
	}

	// Wait briefly to ensure the test database is ready.
	time.Sleep(1 * time.Second)

	// Connect to the newly created test database.
	testConnStr := "host=localhost port=5432 user=postgres dbname=gostgrator_test sslmode=disable"
	testDB, err := sql.Open("pgx", testConnStr)
	if err != nil {
		log.Fatalf("failed to connect to test database: %v", err)
	}
	defer testDB.Close()

	// Create schema in the new test database.
	if _, err := testDB.Exec(`CREATE SCHEMA IF NOT EXISTS gostgrator_schema`); err != nil {
		log.Fatalf("failed to create schema: %v", err)
	}

	// Set up global Postgres config.
	pgTestConfig = gostgrator.Config{
		Driver:            "pg",
		MigrationPattern:  "testdata/migrations/*",
		SchemaTable:       "schemaversion",
		ValidateChecksums: true,
	}

	code := m.Run()

	// Cleanup: reconnect and drop the test database.
	db2, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("failed to reconnect for cleanup: %v", err)
	}
	defer db2.Close()

	// Terminate active connections.
	_, err = db2.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'gostgrator_test'`)
	if err != nil {
		log.Printf("warning: could not terminate connections: %v", err)
	}
	_, err = db2.Exec("DROP DATABASE IF EXISTS gostgrator_test")
	if err != nil {
		log.Printf("failed to drop test database: %v", err)
	}

	os.Exit(code)
}

func TestPostgresMigrations(t *testing.T) {
	ctx := context.Background()
	// Open a connection to the test database.
	connStr := "host=localhost port=5432 user=postgres dbname=gostgrator_test sslmode=disable search_path=gostgrator_schema"
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS schemaversion")
		_ = db.Close()
	}()

	// Create a new Gostgrator instance.
	g, err := gostgrator.NewGostgrator(pgTestConfig, db)
	if err != nil {
		t.Fatalf("failed to create gostgrator: %v", err)
	}

	t.Run("Migrate Up to 003", func(t *testing.T) {
		migs, err := g.Migrate(ctx, "003")
		if err != nil {
			t.Fatalf("Migrate up to 003 failed: %v", err)
		}
		if len(migs) != 3 {
			t.Fatalf("expected 3 migrations, got %d", len(migs))
		}
	})

	t.Run("Migrate to 004 and Database Version", func(t *testing.T) {
		_, err := g.Migrate(ctx, "004")
		if err != nil {
			t.Fatalf("Migrate to 004 failed: %v", err)
		}
		ver, err := g.GetDatabaseVersion(ctx)
		if err != nil {
			t.Fatalf("GetDatabaseVersion failed: %v", err)
		}
		if ver != 4 {
			t.Fatalf("expected database version 4, got %d", ver)
		}
	})

	t.Run("Get Migrations", func (t *testing.T) {
		migs, err := g.GetMigrations()
		if err != nil {
			t.Fatalf("GetMigrations failed: %v", err)
		}
		if len(migs) != 12 {
			t.Fatalf("expected 12 migrations, got %d", len(migs))
		}
		mig := migs[0]

		if (mig.Version != 1) {
			t.Fatalf("expected migration version 1, got %d", mig.Version)
		}

		if (mig.Action != "do") {
			t.Fatalf("expected migration action 'up', got %s", mig.Action)
		}

		// filanem endswith
		if (strings.HasSuffix(mig.Filename, "001_do.sql")) {
			t.Fatalf("expected migration filename '001_do.sql', got %s", mig.Filename)
		}

	})

	t.Run("Get Max Version", func(t *testing.T) {
		max, err := g.GetMaxVersion()
		if err != nil {
			t.Fatalf("GetMaxVersion failed: %v", err)
		}
		// Assuming your testdata/migrations files yield a max version of 6.
		if max != 6 {
			t.Fatalf("expected max version 6, got %d", max)
		}
	})

	t.Run("Run Max Version", func(t *testing.T) {
		_, err := g.Migrate(ctx, "max")
		if err != nil {
			t.Fatalf("Migrate to 004 failed: %v", err)
		}
		ver, err := g.GetDatabaseVersion(ctx)
		if err != nil {
			t.Fatalf("GetDatabaseVersion failed: %v", err)
		}
		if ver != 6 {
			t.Fatalf("expected database version 6, got %d", ver)
		}
	})

	t.Run("Migrate Down to 000", func(t *testing.T) {
		migs, err := g.Migrate(ctx, "000")
		if err != nil {
			t.Fatalf("Migrate down to 000 failed: %v", err)
		}
		// Assuming 4 migrations run on downgrade.
		if len(migs) != 6 {
			t.Fatalf("expected 6 migrations for down, got %d", len(migs))
		}
	})
}

func TestMigrationFail(t *testing.T) {
	t.Run("Migration Failure Handling", func(t *testing.T) {
		ctx := context.Background()

		connStr := "host=localhost port=5432 user=postgres dbname=gostgrator_test sslmode=disable search_path=gostgrator_schema"
		db, err := sql.Open("pgx", connStr)
		if err != nil {
			t.Fatalf("failed to connect to test database: %v", err)
		}

		defer func() {
			_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS schemaversion")
			_ = db.Close()
		}()

		failCfg := pgTestConfig
		failCfg.MigrationPattern = "testdata/failMigrations/*"

		fail, err := gostgrator.NewGostgrator(failCfg, db)
		if err != nil {
			t.Fatalf("failed to create gostgrator for failure test: %v", err)
		}

		_, err = fail.Migrate(ctx, "max")
		if err == nil {
			t.Fatal("expected migration failure error, got none")
		}

		rows, err := db.QueryContext(ctx, "SELECT name FROM widgets")
		if err != nil {
			t.Fatalf("failed to query widgets: %v", err)
		}
		defer rows.Close()

		hasRows := rows.Next()
		if hasRows {
			t.Fatal("expected no rows in widgets table after failed migration")
		}

		t.Run("Migrate Down to 000", func(t *testing.T) {
			migs, err := fail.Migrate(ctx, "000")
			if err != nil {
				t.Fatalf("Migrate down to 000 failed: %v", err)
			}
			// Assuming 4 migrations run on downgrade.
			if len(migs) != 1 {
				t.Fatalf("expected 1 migrations for down, got %d", len(migs))
			}
		})
	})
}

func TestMigrationDupe(t *testing.T) {
	t.Run("Duplicate Migrations Error", func(t *testing.T) {
		ctx := context.Background()

		connStr := "host=localhost port=5432 user=postgres dbname=gostgrator_test sslmode=disable search_path=gostgrator_schema"
		dupCfg := pgTestConfig
		dupCfg.MigrationPattern = "testdata/duplicateMigrations/*"
		dupDB, err := sql.Open("pgx", connStr)
		if err != nil {
			t.Fatalf("failed to connect for duplicate test: %v", err)
		}
		defer func() {
			_, _ = dupDB.ExecContext(ctx, "DROP TABLE IF EXISTS schemaversion")
			_ = dupDB.Close()
		}()
		dup, err := gostgrator.NewGostgrator(dupCfg, dupDB)
		if err != nil {
			t.Fatalf("failed to create gostgrator for duplicate test: %v", err)
		}
		_, err = dup.Migrate(ctx, "")
		if err == nil {
			t.Fatal("expected duplicate migration error, but got none")
		}
	})
}

func TestSqliteMigrations(t *testing.T) {
	ctx := context.Background()
	// Open an in-memory SQLite database.
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite3 in-memory db: %v", err)
	}
	defer db.Close()

	cfg := gostgrator.Config{
		Driver:           "sqlite3",
		MigrationPattern: "testdata/migrations/*",
		SchemaTable:      "versions",
		ValidateChecksums: true,
	}

	g, err := gostgrator.NewGostgrator(cfg, db)
	if err != nil {
		t.Fatalf("failed to create sqlite gostgrator: %v", err)
	}
	defer func() {
		_, _ = g.QueryContext(ctx, "DROP TABLE IF EXISTS versions")
		db.Close()
	}()

	t.Run("Migrate Up to 003", func(t *testing.T) {
		migs, err := g.Migrate(ctx, "003")
		if err != nil {
			t.Fatalf("sqlite migrate up to 003 failed: %v", err)
		}
		if len(migs) != 3 {
			t.Fatalf("expected 3 migrations, got %d", len(migs))
		}
	})

	t.Run("Database Version", func(t *testing.T) {
		_, err := g.Migrate(ctx, "004")
		if err != nil {
			t.Fatalf("sqlite migrate to 004 failed: %v", err)
		}
		ver, err := g.GetDatabaseVersion(ctx)
		if err != nil {
			t.Fatalf("sqlite GetDatabaseVersion failed: %v", err)
		}
		if ver != 4 {
			t.Fatalf("expected database version 4, got %d", ver)
		}
	})

	t.Run("Migrate Down to 000", func(t *testing.T) {
		migs, err := g.Migrate(ctx, "000")
		if err != nil {
			t.Fatalf("sqlite migrate down to 000 failed: %v", err)
		}
		if len(migs) != 4 {
			t.Fatalf("expected 4 migrations for down, got %d", len(migs))
		}
	})
}
