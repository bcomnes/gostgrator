package main_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

var cliBinary string

// Global variables for SQLite testing.
var (
	// We use a temporary file for the SQLite DB.
	testDBFile = filepath.Join(os.TempDir(), "gostgrator_sqlite_test.db")
	// testMigrationsPath: relative path from the integration test package to the test migration files.
	testMigrationsPath = "../../testdata/migrations/*.sql"
)

// TestMain builds the CLI binary and sets up the SQLite test database.
func TestMain(m *testing.M) {
	// Ensure the test database file is removed before starting.
	os.Remove(testDBFile)

	// Build the CLI binary from the parent directory.
	binaryPath := filepath.Join(os.TempDir(), "gostgrator-sqlite-integration")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build SQLite CLI binary: %v\n", err)
		os.Exit(1)
	}
	cliBinary = binaryPath

	code := m.Run()

	// Clean up: remove the database file and the binary.
	os.Remove(testDBFile)
	os.Remove(cliBinary)
	os.Exit(code)
}

// helperRun runs the built CLI binary with the provided arguments and extra environment variables.
func helperRun(args []string, extraEnv ...string) (string, error) {
	cmd := exec.Command(cliBinary, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// For SQLite, the connection string is simply the database file path.
func makeTestConnURL() string {
	return testDBFile
}

// tableExists checks whether a table exists in the SQLite database.
func tableExists(db *sql.DB, name string) (bool, error) {
	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&cnt)
	return cnt > 0, err
}

// -----------------------------------------------------------------------------
// Existing core command tests
// -----------------------------------------------------------------------------

// TestCLIMigrate tests the "migrate" command.
func TestCLIMigrate(t *testing.T) {
	connArg := makeTestConnURL()
	args := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"migrate", "max",
	}
	out, err := helperRun(args)
	if err != nil {
		t.Fatalf("SQLite CLI migrate command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Starting migration") {
		t.Errorf("expected migration start message, got:\n%s", out)
	}
}

// TestCLIDown tests the "down" command.
func TestCLIDown(t *testing.T) {
	connArg := makeTestConnURL()
	args := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"down", "1",
	}
	out, err := helperRun(args)
	if err != nil {
		t.Fatalf("SQLite CLI down command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Rolling back") {
		t.Errorf("expected rollback message, got:\n%s", out)
	}
}

// TestCLIDropSchema tests the "drop-schema" command.
func TestCLIDropSchema(t *testing.T) {
	connArg := makeTestConnURL()
	args := []string{
		"-conn", connArg,
		"drop-schema",
	}
	out, err := helperRun(args)
	if err != nil {
		t.Fatalf("SQLite CLI drop-schema command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Dropping schema table") {
		t.Errorf("expected drop schema message, got:\n%s", out)
	}
}

// TestCLINew tests the "new" command which creates migration files.
func TestCLINew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sqlite_cli_new_integration")
	if err != nil {
		t.Fatalf("failed to create temp migration directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := map[string]interface{}{
		"MigrationPattern": filepath.Join(tmpDir, "*.sql"),
		"Driver":           "sqlite3",
		"SchemaTable":      "schemaversion",
		"ValidateChecksums": true,
	}
	cfgPath := filepath.Join(tmpDir, "config.json")
	cfgFile, err := os.Create(cfgPath)
	if err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}
	enc := json.NewEncoder(cfgFile)
	if err := enc.Encode(cfg); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	cfgFile.Close()

	description := "create test table"
	args := []string{
		"-conn", "dummy",
		"-config", cfgPath,
		"-mode", "int",
		"new", description,
	}
	out, err := helperRun(args)
	if err != nil {
		t.Fatalf("SQLite CLI new command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "New migration created successfully.") {
		t.Errorf("expected new migration success message, got:\n%s", out)
	}

	files, err := filepath.Glob(filepath.Join(tmpDir, "*.sql"))
	if err != nil {
		t.Fatalf("failed to glob migration files: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 migration files, got %d", len(files))
	}
}

// TestFlagOrderingSafe verifies the safeguard against flags placed after positional arguments.
func TestFlagOrderingSafe(t *testing.T) {
	out, _ := helperRun([]string{"migrate", "-conn", "dummy"})
	expected := "Error: Flags must be specified before the command. Please reorder your arguments."
	if !strings.Contains(out, expected) {
		t.Errorf("expected flag ordering error message, got:\n%s", out)
	}
}

// -----------------------------------------------------------------------------
// New precedence & schema-table override tests
// -----------------------------------------------------------------------------

// makeTempConfig file with conn.
func makeTempConfig(conn string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "sqlite_cfg")
	if err != nil {
		return "", nil, err
	}
	p := filepath.Join(dir, "cfg.json")
	f, _ := os.Create(p)
	json.NewEncoder(f).Encode(map[string]string{"conn": conn})
	f.Close()
	return p, func() { os.RemoveAll(dir) }, nil
}

// TestConnPrecedence_ConfigUsed ensures config conn works when flag/env empty.
func TestConnPrecedence_ConfigUsed(t *testing.T) {
	cfgDB := filepath.Join(t.TempDir(), "cfg.db")
	cfgPath, clean, err := makeTempConfig(cfgDB)
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	defer clean()

	_, err = helperRun([]string{"-config", cfgPath, "list"}, "SQLITE_URL=")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, e := os.Stat(cfgDB); e != nil {
		t.Errorf("expected cfg.db to exist")
	}
}

// TestConnPrecedence_EnvWins ensures env beats config.
func TestConnPrecedence_EnvWins(t *testing.T) {
	envDB := filepath.Join(t.TempDir(), "env.db")
	cfgDB := filepath.Join(t.TempDir(), "cfg.db")
	cfgPath, clean, err := makeTempConfig(cfgDB)
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	defer clean()

	_, err = helperRun([]string{"-config", cfgPath, "list"}, "SQLITE_URL="+envDB)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, e := os.Stat(envDB); e != nil {
		t.Errorf("expected env.db to exist")
	}
	if _, e := os.Stat(cfgDB); e == nil {
		t.Errorf("expected cfg.db NOT to be used")
	}
}

// TestConnPrecedence_FlagWins ensures flag beats env+config.
func TestConnPrecedence_FlagWins(t *testing.T) {
	flagDB := filepath.Join(t.TempDir(), "flag.db")
	envDB := filepath.Join(t.TempDir(), "env.db")
	cfgDB := filepath.Join(t.TempDir(), "cfg.db")
	cfgPath, clean, err := makeTempConfig(cfgDB)
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	defer clean()

	_, err = helperRun([]string{"-conn", flagDB, "-config", cfgPath, "list"}, "SQLITE_URL="+envDB)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, e := os.Stat(flagDB); e != nil {
		t.Errorf("expected flag.db to exist")
	}
}

// TestSchemaTableFlagOverridesConfig checks -schema-table overrides config.
func TestSchemaTableFlagOverridesConfig(t *testing.T) {
	conn := filepath.Join(t.TempDir(), "override.db")
	cfg := map[string]any{
		"SchemaTable": "cfg_table",
		"conn":        conn,
	}
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "cfg.json")
	cf, _ := os.Create(cfgPath)
	json.NewEncoder(cf).Encode(cfg)
	cf.Close()

	flagTable := "flag_table"

	_, err := helperRun([]string{"-config", cfgPath, "-schema-table", flagTable, "migrate", "max"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	db, err := sql.Open("sqlite3", conn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	okFlag, _ := tableExists(db, flagTable)
	okCfg, _ := tableExists(db, "cfg_table")

	if !okFlag || okCfg {
		t.Errorf("expected only %s to exist, got flag=%v cfg=%v", flagTable, okFlag, okCfg)
	}
}

// -----------------------------------------------------------------------------
// List-chain test (reset → up → down) remains unchanged
// -----------------------------------------------------------------------------

// TestCLIListChain tests the "list" command by chaining operations: reset → up → down.
func TestCLIListChain(t *testing.T) {
	connArg := makeTestConnURL()

	// Reset: migrate down to 0 to clear any previous state.
	resetArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "migrate", "0"}
	_, err := helperRun(resetArgs)
	if err != nil {
		t.Fatalf("SQLite CLI reset (migrate 0) failed: %v", err)
	}

	listArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "list"}

	// Step 1: List after reset; expect current version 0.
	out, err := helperRun(listArgs)
	if err != nil {
		t.Fatalf("SQLite CLI list command (initial) failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version: 0") {
		t.Errorf("expected current migration version 0 initially, got:\n%s", out)
	}

	// Step 2: Migrate to max (assumed max version is 6), then list.
	migrateArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "migrate", "max"}
	_, err = helperRun(migrateArgs)
	if err != nil {
		t.Fatalf("SQLite CLI migrate command failed: %v", err)
	}
	out, err = helperRun(listArgs)
	if err != nil {
		t.Fatalf("SQLite CLI list command (after migrate) failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version: 6") {
		t.Errorf("expected current migration version 6 after migrate, got:\n%s", out)
	}
	if !strings.Contains(out, "Version 6:") || !strings.Contains(out, "<== current") {
		t.Errorf("expected migration version 6 to be annotated as current, got:\n%s", out)
	}

	// Step 3: down 1 then list; expect version 5.
	downArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "down", "1"}
	_, err = helperRun(downArgs)
	if err != nil {
		t.Fatalf("SQLite CLI down command failed: %v", err)
	}
	out, err = helperRun(listArgs)
	if err != nil {
		t.Fatalf("SQLite CLI list command (after down) failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version: 5") {
		t.Errorf("expected current migration version 5 after down, got:\n%s", out)
	}
	if !strings.Contains(out, "Version 5:") || !strings.Contains(out, "<== current") {
		t.Errorf("expected migration version 5 to be annotated as current, got:\n%s", out)
	}
}
