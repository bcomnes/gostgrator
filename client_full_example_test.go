package gostgrator_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing/fstest"

	"github.com/bcomnes/gostgrator/v2"
	_ "modernc.org/sqlite"
)

// sqliteClient implements its own bookkeeping rather than wrapping a built-in client.
// Identifiers are fixed, quoted SQL; all variable values are bound parameters.
// It owns a fixed tracking table; it does not upgrade legacy tracking schemas.
type sqliteClient struct {
	db *sql.DB
}

var _ gostgrator.Client = (*sqliteClient)(nil)

func (c *sqliteClient) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, query, args...)
}

func (c *sqliteClient) ExecContext(ctx context.Context, script string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, script, args...)
}

func (c *sqliteClient) GetDatabaseVersionSql() gostgrator.Statement {
	return gostgrator.Statement{SQL: `SELECT COALESCE(MAX(version), 0) FROM "example_migrations"`}
}

func (c *sqliteClient) HasVersionTable(ctx context.Context) (bool, error) {
	var exists bool
	err := c.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master
			WHERE type = ? AND name = ?
		)
	`, "table", "example_migrations").Scan(&exists)
	return exists, err
}

func (c *sqliteClient) EnsureTable(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS "example_migrations" (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			md5 TEXT NOT NULL,
			run_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (c *sqliteClient) GetMd5Sql(m gostgrator.Migration) gostgrator.Statement {
	return gostgrator.Statement{SQL: `SELECT md5 FROM "example_migrations" WHERE version = ?`, Args: []any{m.Version}}
}

func (c *sqliteClient) PersistActionSql(m gostgrator.Migration) gostgrator.Statement {
	if m.Action == "undo" {
		return gostgrator.Statement{SQL: `DELETE FROM "example_migrations" WHERE version = ?`, Args: []any{m.Version}}
	}
	return gostgrator.Statement{
		SQL:  `INSERT INTO "example_migrations" (version, name, md5) VALUES (?, ?, ?)`,
		Args: []any{m.Version, m.Name, m.Md5},
	}
}

// This example implements every Client method directly against database/sql.
// A new dialect supplies its own tracking-table DDL and bookkeeping queries.
func ExampleClient() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close() // The caller owns the connection, not Gostgrator.
	db.SetMaxOpenConns(1)

	client := &sqliteClient{db: db}
	g, err := gostgrator.NewGostgrator(gostgrator.Config{
		Migrations: gostgrator.FSMigrations{
			FS: fstest.MapFS{
				"001.do.create-items.sql":   {Data: []byte("CREATE TABLE items (id INTEGER PRIMARY KEY);")},
				"001.undo.create-items.sql": {Data: []byte("DROP TABLE items;")},
			},
			Pattern: "*.sql",
		},
		ValidateChecksums: true,
	}, client)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	applied, err := g.Migrate(ctx, "max")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Applied %d migration\n", len(applied))

	// Re-running validates the stored checksum without applying anything again.
	applied, err = g.Migrate(ctx, "max")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Applied %d migrations on rerun\n", len(applied))

	undone, err := g.Migrate(ctx, "0")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Undid %d migration\n", len(undone))
	version, err := g.GetDatabaseVersion(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Database version: %d\n", version)

	// Output:
	// Applied 1 migration
	// Applied 0 migrations on rerun
	// Undid 1 migration
	// Database version: 0
}
