package gostgrator

import (
	"context"
	"database/sql"
	"io/fs"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

func sourceSQLite(t *testing.T, cfg Config) (*Gostgrator, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	cfg.Driver = "sqlite"
	g, err := NewGostgrator(cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	return g, db
}

func TestMigrationSourceSQLiteUpDown(t *testing.T) {
	for name, source := range map[string]MigrationSource{
		"embed": FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"},
		"disk":  DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"},
	} {
		t.Run(name, func(t *testing.T) {
			g, db := sourceSQLite(t, Config{Migrations: source})
			ctx := context.Background()
			up, err := g.Migrate(ctx, "max")
			if err != nil {
				t.Fatal(err)
			}
			if len(up) != 2 || up[0].Version != 1 || up[1].Version != 2 || up[0].Action != "do" || up[1].Action != "do" {
				t.Fatalf("unexpected up migrations: %+v", up)
			}
			if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 2 {
				t.Fatalf("version = %d, %v; want 2", version, err)
			}
			var name string
			if err := db.QueryRow("SELECT name FROM source_items WHERE id = 1").Scan(&name); err != nil || name != "embedded" {
				t.Fatalf("seeded name = %q, %v", name, err)
			}
			for _, m := range up {
				var checksum string
				if err := db.QueryRow("SELECT md5 FROM schemaversion WHERE version = ?", m.Version).Scan(&checksum); err != nil || checksum != m.Md5 || checksum == "" {
					t.Fatalf("persisted checksum = %q, %v; want %q", checksum, err, m.Md5)
				}
			}
			if err := g.ValidateMigrations(ctx, 2); err != nil {
				t.Fatal(err)
			}
			if again, err := g.Migrate(ctx, "max"); err != nil || len(again) != 0 {
				t.Fatalf("repeat migration = %v, %v", again, err)
			}
			down, err := g.Migrate(ctx, "0")
			if err != nil {
				t.Fatal(err)
			}
			if len(down) != 2 || down[0].Version != 2 || down[1].Version != 1 || down[0].Action != "undo" || down[1].Action != "undo" {
				t.Fatalf("unexpected down migrations: %+v", down)
			}
			if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 0 {
				t.Fatalf("version = %d, %v; want 0", version, err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'source_items'").Scan(&count); err != nil || count != 0 {
				t.Fatalf("table count after rollback = %d, %v", count, err)
			}
		})
	}
}

func TestMigrationSourceRunMigrationsRetainsFS(t *testing.T) {
	// The runner has a different source, so only the returned Migration can
	// supply the embedded SQL (the fs.Sub paths do not exist on disk).
	loader, _ := sourceSQLite(t, Config{Migrations: FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"}})
	migs, err := loader.GetMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var up []Migration
	for _, m := range migs {
		if m.Action == "do" {
			up = append(up, m)
		}
	}
	sortMigrationsAsc(up)
	runner, db := sourceSQLite(t, Config{Migrations: FSMigrations{FS: fstest.MapFS{}, Pattern: "*.sql"}})
	ctx := context.Background()
	if err := runner.client.EnsureTable(ctx); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.RunMigrations(ctx, up)
	if err != nil || len(applied) != 2 {
		t.Fatalf("RunMigrations = %v, %v; want two applied migrations", applied, err)
	}
	var name string
	if err := db.QueryRow("SELECT name FROM source_items WHERE id = 1").Scan(&name); err != nil || name != "embedded" {
		t.Fatalf("seeded name = %q, %v", name, err)
	}
}

func TestMigrationSourceChecksumDrift(t *testing.T) {
	files := fstest.MapFS{"001.do.sql": {Data: []byte("CREATE TABLE source_drift (id INTEGER);\r\n")}}
	g, db := sourceSQLite(t, Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}, Newline: "LF", ValidateChecksums: true})
	ctx := context.Background()
	if _, err := g.Migrate(ctx, "max"); err != nil {
		t.Fatal(err)
	}
	files["001.do.sql"].Data = []byte("CREATE TABLE source_drift (id INTEGER);\n")
	if err := g.ValidateMigrations(ctx, 1); err != nil {
		t.Fatalf("normalized newline change failed validation: %v", err)
	}
	files["001.do.sql"].Data = []byte("CREATE TABLE source_drift (id TEXT);\n")
	if err := g.ValidateMigrations(ctx, 1); err == nil {
		t.Fatal("changed SQL passed checksum validation")
	}
	files["002.do.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE should_not_run (id INTEGER);")}
	if _, err := g.Migrate(ctx, "max"); err == nil {
		t.Fatal("Migrate accepted checksum drift")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'should_not_run'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("pending migration ran despite checksum drift: count = %d, error = %v", count, err)
	}
}

func TestMigrationSourceRunMigrationsReadError(t *testing.T) {
	files := fstest.MapFS{"001.do.sql": {Data: []byte("SELECT 1;")}}
	g, _ := sourceSQLite(t, Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}})
	migs, err := g.GetMigrations()
	if err != nil || len(migs) != 1 {
		t.Fatalf("GetMigrations = %v, %v", migs, err)
	}
	ctx := context.Background()
	if err := g.client.EnsureTable(ctx); err != nil {
		t.Fatal(err)
	}
	delete(files, "001.do.sql")
	if applied, err := g.RunMigrations(ctx, migs); err == nil || len(applied) != 0 {
		t.Fatalf("RunMigrations = %v, %v; want read error and no applied migrations", applied, err)
	}
	if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 0 {
		t.Fatalf("version after read failure = %d, %v", version, err)
	}
}

var _ fs.FS = sourceFixtures
