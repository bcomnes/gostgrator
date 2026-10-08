package gostgrator_test

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bcomnes/gostgrator/v2"
)

// Also verifies that Gostgrator forwards arguments from custom version builders.
type statementClient struct {
	gostgrator.Client
	scripts []string
	args    [][]any
}

func (c *statementClient) GetDatabaseVersionSql() gostgrator.Statement {
	stmt := c.Client.GetDatabaseVersionSql()
	stmt.SQL = strings.Replace(stmt.SQL, "SELECT version", "SELECT version + $1", 1)
	stmt.Args = []any{0}
	return stmt
}

func (c *statementClient) ExecContext(ctx context.Context, script string, args ...any) (sql.Result, error) {
	c.scripts = append(c.scripts, script)
	c.args = append(c.args, append([]any(nil), args...))
	return c.Client.ExecContext(ctx, script, args...)
}

func TestBookkeepingStatements(t *testing.T) {
	for _, dialect := range []string{"sqlite", "pg"} {
		t.Run(dialect, func(t *testing.T) {
			for _, table := range []string{`versions '"; --`, `schema '"; --.versions '"; --`} {
				t.Run(table, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					driver, conn := "sqlite", ":memory:"
					if dialect == "pg" {
						driver, conn = "pgx", "host=localhost port=5432 user=postgres dbname=gostgrator_test sslmode=disable"
					}
					db, err := sql.Open(driver, conn)
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					db.SetMaxOpenConns(1)
					cfg := gostgrator.Config{Driver: dialect, SchemaTable: table}
					client, err := gostgrator.NewClient(cfg, db)
					if err != nil {
						t.Fatal(err)
					}
					// Expected identifier syntax is independent of the implementation.
					quoted := `"versions '""; --"`
					if strings.Contains(table, ".") {
						quoted = `"schema '""; --.versions '""; --"`
						if dialect == "pg" {
							quoted = `"schema '""; --"."versions '""; --"`
						}
					}
					defer db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoted)
					if exists, err := client.HasVersionTable(ctx); err != nil || exists {
						t.Fatalf("initial table exists = %t, %v", exists, err)
					}
					if err := client.EnsureTable(ctx); err != nil {
						t.Fatal(err)
					}
					if err := client.EnsureTable(ctx); err != nil {
						t.Fatalf("repeat ensure: %v", err)
					}
					if exists, err := client.HasVersionTable(ctx); err != nil || !exists {
						t.Fatalf("table exists = %t, %v", exists, err)
					}

					m := gostgrator.Migration{Version: 17, Action: "DO", Name: "O'Brien'); DELETE FROM sentinel; --", Md5: "hash'); DROP TABLE sentinel; --"}
					stmt := client.PersistActionSql(m)
					if strings.Contains(stmt.SQL, m.Name) || strings.Contains(stmt.SQL, m.Md5) || len(stmt.Args) != 4 || !reflect.DeepEqual(stmt.Args[:3], []any{m.Version, m.Name, m.Md5}) {
						t.Fatalf("unbound insert: %+v", stmt)
					}
					placeholder := "?"
					if dialect == "pg" {
						placeholder = "$1"
					}
					if !strings.Contains(stmt.SQL, "VALUES ("+placeholder) {
						t.Fatalf("wrong placeholders: %+v", stmt)
					}
					if _, err := client.ExecContext(ctx, stmt.SQL, stmt.Args...); err != nil {
						t.Fatal(err)
					}
					var name, checksum string
					if err := db.QueryRowContext(ctx, "SELECT name, md5 FROM "+quoted+" WHERE version = "+placeholder, m.Version).Scan(&name, &checksum); err != nil || name != m.Name || checksum != m.Md5 {
						t.Fatalf("stored values = %q, %q, %v", name, checksum, err)
					}
					stmt = client.GetMd5Sql(m)
					if !reflect.DeepEqual(stmt.Args, []any{m.Version}) {
						t.Fatalf("checksum args = %v", stmt.Args)
					}
					if err := db.QueryRowContext(ctx, stmt.SQL, stmt.Args...).Scan(&checksum); err != nil || checksum != m.Md5 {
						t.Fatalf("checksum = %q, %v", checksum, err)
					}
					g, err := gostgrator.NewGostgrator(gostgrator.Config{}, client)
					if err != nil {
						t.Fatal(err)
					}
					if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 17 {
						t.Fatalf("version = %d, %v", version, err)
					}
					m.Action = "UNDO"
					stmt = client.PersistActionSql(m)
					if !reflect.DeepEqual(stmt.Args, []any{m.Version}) {
						t.Fatalf("undo args = %v", stmt.Args)
					}
					if _, err := client.ExecContext(ctx, stmt.SQL, stmt.Args...); err != nil {
						t.Fatal(err)
					}
					if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 0 {
						t.Fatalf("undo version = %d, %v", version, err)
					}

					wrapped := &statementClient{Client: client}
					upSQL := "CREATE TABLE statement_items (id INTEGER); INSERT INTO statement_items VALUES (1);"
					downSQL := "DELETE FROM statement_items; DROP TABLE statement_items;"
					files := fstest.MapFS{
						"001.do.O'Brien; --.sql":   {Data: []byte(upSQL)},
						"001.undo.O'Brien; --.sql": {Data: []byte(downSQL)},
					}
					g, err = gostgrator.NewGostgrator(gostgrator.Config{Migrations: gostgrator.FSMigrations{FS: files, Pattern: "*.sql"}, ValidateChecksums: true}, wrapped)
					if err != nil {
						t.Fatal(err)
					}
					if up, err := g.Migrate(ctx, "max"); err != nil || len(up) != 1 {
						t.Fatalf("up = %v, %v", up, err)
					}
					if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 1 {
						t.Fatalf("up version = %d, %v", version, err)
					}
					rows, err := g.QueryContext(ctx, "SELECT id FROM statement_items WHERE id = "+placeholder, 1)
					if err != nil {
						t.Fatal(err)
					}
					if !rows.Next() {
						rows.Close()
						t.Fatal("bound public query returned no rows")
					}
					rows.Close()
					if again, err := g.Migrate(ctx, "max"); err != nil || len(again) != 0 {
						t.Fatalf("rerun = %v, %v", again, err)
					}
					files["001.do.O'Brien; --.sql"].Data = []byte(upSQL + " -- changed")
					if err := g.ValidateMigrations(ctx, 1); err == nil || !strings.Contains(err.Error(), "MD5 checksum failed") {
						t.Fatalf("checksum drift = %v", err)
					}
					files["001.do.O'Brien; --.sql"].Data = []byte(upSQL)
					if down, err := g.Down(ctx, 1); err != nil || len(down) != 1 {
						t.Fatalf("down = %v, %v", down, err)
					}
					if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 0 {
						t.Fatalf("down version = %d, %v", version, err)
					}
					if len(wrapped.scripts) != 4 || wrapped.scripts[0] != upSQL || wrapped.scripts[2] != downSQL || len(wrapped.args[0]) != 0 || len(wrapped.args[2]) != 0 || len(wrapped.args[1]) != 4 || len(wrapped.args[3]) != 1 {
						t.Fatalf("execution calls = %v, args = %v", wrapped.scripts, wrapped.args)
					}
				})
			}
		})
	}
}

func TestPostgresBookkeepingSearchPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", "host=localhost port=5432 user=postgres dbname=gostgrator_test sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// The earlier visible table needs upgrading. A same-named table in another
	// schema must not supply its columns, nor imply existence when not visible.
	for _, query := range []string{
		`CREATE SCHEMA statement_first`,
		`CREATE SCHEMA statement_second`,
		`CREATE TABLE statement_first."Search '"" Versions" (version BIGINT PRIMARY KEY)`,
		`CREATE TABLE statement_second."Search '"" Versions" (version BIGINT PRIMARY KEY, name TEXT, md5 TEXT, run_at TIMESTAMPTZ)`,
		`SET search_path TO statement_first, statement_second`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	defer db.ExecContext(ctx, `DROP SCHEMA statement_first CASCADE; DROP SCHEMA statement_second CASCADE`)
	client := gostgrator.NewPostgresClient(gostgrator.Config{SchemaTable: `Search '" Versions`}, db)
	if err := client.EnsureTable(ctx); err != nil {
		t.Fatal(err)
	}
	stmt := client.PersistActionSql(gostgrator.Migration{Version: 1, Action: "do", Name: "first", Md5: "checksum"})
	if _, err := client.ExecContext(ctx, stmt.SQL, stmt.Args...); err != nil {
		t.Fatalf("upgraded visible table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE statement_first."Search '"" Versions"; SET search_path TO statement_first`); err != nil {
		t.Fatal(err)
	}
	if exists, err := client.HasVersionTable(ctx); err != nil || exists {
		t.Fatalf("non-visible table exists = %t, %v", exists, err)
	}
	if err := client.EnsureTable(ctx); err != nil {
		t.Fatalf("create in current schema: %v", err)
	}
	if _, err := client.ExecContext(ctx, stmt.SQL, stmt.Args...); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM statement_second."Search '"" Versions"`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("other schema changed: %d, %v", count, err)
	}
}
