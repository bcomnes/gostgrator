package gostgrator

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	_ "modernc.org/sqlite"
)

// Calls made inside the wrapped client are deliberately not recorded: these
// observations prove the orchestrator uses the supplied Client interface.
type injectionClient struct {
	Client
	calls     []string
	scripts   []string
	persisted []Migration
	checksums []Migration
	fail      string
	err       error
}

func (c *injectionClient) record(method string) error {
	c.calls = append(c.calls, method)
	if c.fail == method {
		return c.err
	}
	return nil
}

func (c *injectionClient) QueryContext(ctx context.Context, query string) (*sql.Rows, error) {
	method := "query"
	if strings.Contains(query, "SELECT md5") {
		method = "checksum query"
	}
	if err := c.record(method); err != nil {
		return nil, err
	}
	return c.Client.QueryContext(ctx, query)
}

func (c *injectionClient) ExecContext(ctx context.Context, script string) (sql.Result, error) {
	c.scripts = append(c.scripts, script)
	method := "execute"
	if strings.Contains(script, "injected_versions") {
		method = "bookkeeping"
	}
	if err := c.record(method); err != nil {
		return nil, err
	}
	return c.Client.ExecContext(ctx, script)
}

func (c *injectionClient) EnsureTable(ctx context.Context) error {
	if err := c.record("ensure"); err != nil {
		return err
	}
	return c.Client.EnsureTable(ctx)
}

func (c *injectionClient) HasVersionTable(ctx context.Context) (bool, error) {
	if err := c.record("has table"); err != nil {
		return false, err
	}
	return c.Client.HasVersionTable(ctx)
}

func (c *injectionClient) GetDatabaseVersionSql() string {
	c.calls = append(c.calls, "version SQL")
	return c.Client.GetDatabaseVersionSql()
}

func (c *injectionClient) GetMd5Sql(m Migration) string {
	c.checksums = append(c.checksums, m)
	return c.Client.GetMd5Sql(m)
}

func (c *injectionClient) PersistActionSql(m Migration) string {
	c.persisted = append(c.persisted, m)
	return c.Client.PersistActionSql(m)
}

func injectionSQLite(t *testing.T) (*injectionClient, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return &injectionClient{Client: NewSqlite3Client(Config{
		Driver: "sqlite", SchemaTable: "injected_versions",
	}, db)}, db
}

func TestClientInjectionConstructor(t *testing.T) {
	for _, driver := range []string{"", "unsupported", "pg", "sqlite"} {
		t.Run("driver="+driver, func(t *testing.T) {
			client, _ := injectionSQLite(t)
			cfg := Config{
				Driver: driver, Conn: "not a database connection", SchemaTable: "misleading_versions",
				Migrations: FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"}, Newline: "LF",
			}
			g, err := NewGostgratorWithClient(cfg, client)
			if err != nil {
				t.Fatal(err)
			}
			if g.client != client {
				t.Fatal("constructor replaced the supplied client")
			}
			if len(client.calls) != 0 {
				t.Fatalf("constructor performed database work: %v", client.calls)
			}
			if cfg.ValidateChecksums {
				t.Fatal("constructor mutated caller configuration")
			}
			want := cfg
			want.ValidateChecksums = DefaultConfig.ValidateChecksums
			if !reflect.DeepEqual(g.cfg, want) {
				t.Fatalf("normalized config = %+v, want %+v", g.cfg, want)
			}
			if err := g.client.EnsureTable(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := client.Client.(*Sqlite3Client).cfg.SchemaTable; got != "injected_versions" {
				t.Fatalf("injected schema changed to %q", got)
			}
		})
	}
}

func TestClientInjectionRejectsNil(t *testing.T) {
	var sqlite *Sqlite3Client
	var custom *injectionClient
	for name, client := range map[string]Client{"nil": nil, "typed nil built-in": sqlite, "typed nil custom": custom} {
		t.Run(name, func(t *testing.T) {
			g, err := NewGostgratorWithClient(Config{Migrations: DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"}}, client)
			if err == nil || g != nil {
				t.Fatalf("constructor = %v, %v; want nil and error", g, err)
			}
		})
	}
}

func TestClientInjectionRejectsLegacyPattern(t *testing.T) {
	client, db := injectionSQLite(t)
	cfg := Config{Driver: "sqlite", MigrationPattern: "testdata/source_sqlite/*.sql"}
	g, err := NewGostgratorWithClient(cfg, client)
	if g != nil || err == nil || !strings.Contains(err.Error(), "use Config.Migrations") {
		t.Fatalf("constructor = %v, %v; want error directing callers to Config.Migrations", g, err)
	}
	if len(client.calls) != 0 {
		t.Fatalf("constructor performed database work: %v", client.calls)
	}
	if _, err := NewGostgrator(cfg, db); err != nil {
		t.Fatalf("existing constructor must retain legacy support: %v", err)
	}
}

func TestClientInjectionConfigParity(t *testing.T) {
	files := sourceFixtureFS(t)
	var disk *DiskMigrations
	var virtual *FSMigrations
	var nilFS *embed.FS
	cases := map[string]struct {
		cfg     Config
		invalid bool
	}{
		"defaults": {cfg: Config{}},

		"disk":             {cfg: Config{Migrations: DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"}}},
		"disk pointer":     {cfg: Config{Migrations: &DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"}}},
		"FS":               {cfg: Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}}},
		"FS pointer":       {cfg: Config{Migrations: &FSMigrations{FS: files, Pattern: "*.sql"}}},
		"explicit options": {cfg: Config{SchemaTable: "explicit_versions", Newline: "CRLF", ValidateChecksums: true}},
		"empty disk":       {cfg: Config{Migrations: DiskMigrations{}}, invalid: true},
		"empty FS pattern": {cfg: Config{Migrations: FSMigrations{FS: files}}, invalid: true},
		"nil FS":           {cfg: Config{Migrations: FSMigrations{Pattern: "*.sql"}}, invalid: true},
		"typed nil FS":     {cfg: Config{Migrations: FSMigrations{FS: nilFS, Pattern: "*.sql"}}, invalid: true},
		"nil disk pointer": {cfg: Config{Migrations: disk}, invalid: true},
		"nil FS pointer":   {cfg: Config{Migrations: virtual}, invalid: true},
		"disk conflict":    {cfg: Config{MigrationPattern: "*.sql", Migrations: DiskMigrations{Pattern: "*.sql"}}, invalid: true},
		"FS conflict":      {cfg: Config{MigrationPattern: "*.sql", Migrations: FSMigrations{FS: files, Pattern: "*.sql"}}, invalid: true},
		"malformed disk":   {cfg: Config{Migrations: DiskMigrations{Pattern: "["}}, invalid: true},
		"malformed FS":     {cfg: Config{Migrations: FSMigrations{FS: files, Pattern: "["}}, invalid: true},
	}
	for _, pattern := range []string{"./*.sql", "../*.sql", "/migrations/*.sql", "migrations/../*.sql", "migrations/./*.sql"} {
		cases[pattern] = struct {
			cfg     Config
			invalid bool
		}{Config{Migrations: FSMigrations{FS: files, Pattern: pattern}}, true}
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client, db := injectionSQLite(t)
			cfg := tc.cfg
			cfg.Driver = "sqlite"
			builtIn, builtInErr := NewGostgrator(cfg, db)
			injected, injectedErr := NewGostgratorWithClient(cfg, client)
			if tc.invalid {
				if builtInErr == nil || injectedErr == nil || builtIn != nil || injected != nil {
					t.Fatalf("invalid source: built-in = %v, %v; injected = %v, %v", builtIn, builtInErr, injected, injectedErr)
				}
				if builtInErr.Error() != injectedErr.Error() {
					t.Fatalf("source validation differs: %v versus %v", builtInErr, injectedErr)
				}
				return
			}
			if builtInErr != nil || injectedErr != nil {
				t.Fatalf("valid source: built-in error = %v; injected error = %v", builtInErr, injectedErr)
			}
			if !reflect.DeepEqual(builtIn.cfg, injected.cfg) {
				t.Fatalf("normalized configs differ: %+v versus %+v", builtIn.cfg, injected.cfg)
			}
			if injected.cfg.SchemaTable == "" || injected.cfg.ValidateChecksums != DefaultConfig.ValidateChecksums {
				t.Fatalf("missing defaults: %+v", injected.cfg)
			}
		})
	}
}

func TestClientInjectionBuiltInSchemaDefault(t *testing.T) {
	_, db := injectionSQLite(t)
	g, err := NewGostgrator(Config{Driver: "sqlite", MigrationPattern: "testdata/source_sqlite/*.sql"}, db)
	if err != nil {
		t.Fatal(err)
	}
	client, ok := g.client.(*Sqlite3Client)
	if !ok {
		t.Fatalf("client type = %T, want *Sqlite3Client", g.client)
	}
	if client.cfg.SchemaTable != DefaultConfig.SchemaTable {
		t.Fatalf("client created with schema %q, want %q", client.cfg.SchemaTable, DefaultConfig.SchemaTable)
	}
	if _, err := g.Migrate(context.Background(), "max"); err != nil {
		t.Fatalf("migration using default schema: %v", err)
	}
	var version int
	if err := db.QueryRow("SELECT max(version) FROM schemaversion").Scan(&version); err != nil || version != 2 {
		t.Fatalf("default schema version = %d, %v; want 2", version, err)
	}
}

func TestClientInjectionMigrations(t *testing.T) {
	for name, cfg := range map[string]Config{

		"disk": {Migrations: DiskMigrations{Pattern: "testdata/source_sqlite/*.sql"}},
		"FS":   {Migrations: FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"}},
	} {
		t.Run(name, func(t *testing.T) {
			client, db := injectionSQLite(t)
			cfg.Driver, cfg.Conn, cfg.SchemaTable = "unsupported", "invalid connection", "misleading_versions"
			g, err := NewGostgratorWithClient(cfg, client)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 0 {
				t.Fatalf("initial version = %d, %v", version, err)
			}
			up, err := g.Migrate(ctx, "max")
			if err != nil || len(up) != 2 {
				t.Fatalf("up = %v, %v; want two migrations", up, err)
			}
			if up[0].Version != 1 || up[1].Version != 2 || up[0].Action != "do" || up[1].Action != "do" {
				t.Fatalf("unexpected up order: %+v", up)
			}
			if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 2 {
				t.Fatalf("up version = %d, %v", version, err)
			}
			rows, err := g.QueryContext(ctx, "SELECT name FROM source_items WHERE id = 1")
			if err != nil {
				t.Fatal(err)
			}
			var seeded string
			if !rows.Next() {
				rows.Close()
				t.Fatal("migration did not seed source_items")
			}
			err = rows.Scan(&seeded)
			rows.Close()
			if err != nil || seeded != "embedded" {
				t.Fatalf("seed = %q, %v", seeded, err)
			}
			for _, m := range up {
				var checksum, name string
				if err := db.QueryRow("SELECT md5, name FROM injected_versions WHERE version = ?", m.Version).Scan(&checksum, &name); err != nil || checksum == "" || checksum != m.Md5 || name != m.Name {
					t.Fatalf("bookkeeping for %d: checksum = %q, name = %q, error = %v", m.Version, checksum, name, err)
				}
			}
			if err := g.ValidateMigrations(ctx, 2); err != nil {
				t.Fatal(err)
			}
			if len(client.checksums) != 2 {
				t.Fatalf("checksum SQL calls = %d, want 2", len(client.checksums))
			}
			if again, err := g.Migrate(ctx, "max"); err != nil || len(again) != 0 {
				t.Fatalf("repeat = %v, %v", again, err)
			}
			down, err := g.Down(ctx, 2)
			if err != nil || len(down) != 2 {
				t.Fatalf("down = %v, %v", down, err)
			}
			if down[0].Version != 2 || down[1].Version != 1 || down[0].Action != "undo" || down[1].Action != "undo" {
				t.Fatalf("unexpected down order: %+v", down)
			}
			if version, err := g.GetDatabaseVersion(ctx); err != nil || version != 0 {
				t.Fatalf("down version = %d, %v", version, err)
			}
			if len(client.persisted) != 4 || len(client.scripts) != 8 {
				t.Fatalf("persist calls = %d, executions = %d; want 4 and 8", len(client.persisted), len(client.scripts))
			}
			for i, m := range append(up, down...) {
				if got := client.persisted[i]; got.Version != m.Version || got.Action != m.Action {
					t.Errorf("persist call %d = %+v, want %+v", i, got, m)
				}
				script, err := m.getSQL()
				if err != nil || client.scripts[2*i] != script || !strings.Contains(client.scripts[2*i+1], "injected_versions") {
					t.Errorf("execution pair %d did not run migration then injected bookkeeping: %v", i, err)
				}
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('source_items', 'misleading_versions', 'schemaversion')").Scan(&count); err != nil || count != 0 {
				t.Fatalf("unexpected tables after rollback: %d, %v", count, err)
			}
			if err := db.QueryRow("SELECT count(*) FROM injected_versions WHERE version > 0").Scan(&count); err != nil || count != 0 {
				t.Fatalf("remaining bookkeeping rows = %d, %v", count, err)
			}
		})
	}
}

func TestClientInjectionChecksumDrift(t *testing.T) {
	files := fstest.MapFS{"001.do.sql": {Data: []byte("CREATE TABLE injected_drift (id INTEGER);\r\n")}}
	client, db := injectionSQLite(t)
	g, err := NewGostgratorWithClient(Config{Migrations: FSMigrations{FS: files, Pattern: "*.sql"}, Newline: "LF", SchemaTable: "misleading_versions"}, client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := g.Migrate(ctx, "max"); err != nil {
		t.Fatal(err)
	}
	files["001.do.sql"].Data = []byte("CREATE TABLE injected_drift (id INTEGER);\n")
	if err := g.ValidateMigrations(ctx, 1); err != nil {
		t.Fatalf("newline normalization: %v", err)
	}
	files["001.do.sql"].Data = []byte("CREATE TABLE injected_drift (id TEXT);\n")
	if err := g.ValidateMigrations(ctx, 1); err == nil || !strings.Contains(err.Error(), "MD5 checksum failed") {
		t.Fatalf("checksum drift error = %v", err)
	}
	files["002.do.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE should_not_run (id INTEGER);")}
	before := len(client.scripts)
	if applied, err := g.Migrate(ctx, "max"); err == nil || !strings.Contains(err.Error(), "MD5 checksum failed") || len(applied) != 0 {
		t.Fatalf("migration with drift = %v, %v", applied, err)
	}
	if len(client.scripts) != before {
		t.Fatal("executed SQL despite checksum drift")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'should_not_run'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("pending table count = %d, %v", count, err)
	}
}

func TestClientInjectionErrors(t *testing.T) {
	for _, failure := range []string{"ensure", "has table", "query", "checksum query", "execute", "bookkeeping"} {
		t.Run(failure, func(t *testing.T) {
			client, _ := injectionSQLite(t)
			g, err := NewGostgratorWithClient(Config{Migrations: FSMigrations{FS: sourceFixtureFS(t), Pattern: "*.sql"}}, client)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if failure == "checksum query" {
				if _, err := g.Migrate(ctx, "1"); err != nil {
					t.Fatal(err)
				}
			}
			before := len(client.scripts)
			sentinel := errors.New("injected " + failure + " failure")
			client.fail, client.err = failure, sentinel
			applied, err := g.Migrate(ctx, "max")
			if !errors.Is(err, sentinel) || len(applied) != 0 {
				t.Fatalf("Migrate = %v, %v; want no applied migrations and %v", applied, err, sentinel)
			}
			wantExecutions := 0
			if failure == "execute" {
				wantExecutions = 1
			} else if failure == "bookkeeping" {
				wantExecutions = 2
			}
			if got := len(client.scripts) - before; got != wantExecutions {
				t.Fatalf("executions after failure injection = %d, want %d", got, wantExecutions)
			}
			if failure == "query" {
				if rows, err := g.QueryContext(ctx, "SELECT 1"); !errors.Is(err, sentinel) || rows != nil {
					t.Fatalf("QueryContext = %v, %v; want injected error", rows, err)
				}
			}
			client.fail = ""
			wantVersion := 0
			if failure == "checksum query" {
				wantVersion = 1
			}
			if version, err := g.GetDatabaseVersion(ctx); err != nil || version != wantVersion {
				t.Fatalf("version after failure = %d, %v; want %d", version, err, wantVersion)
			}
		})
	}
}
