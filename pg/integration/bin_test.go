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
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var cliBinary string

// Global variables for test database and migration files.
var (
	testDBName  = "gostgrator_cli_test"
	testSchema  = "gostgrator_schema"
	// Base connection string for DSN-based connections used in TestMain.
	baseConnStr = "host=localhost port=5432 user=postgres sslmode=disable"
	// testMigrationsPath: relative path from the integration test package to the test migration files.
	testMigrationsPath = "../../testdata/migrations/*.sql"
)

// TestMain sets up the test database (using DSN style) and builds the CLI binary before running tests,
// then cleans up afterward.
func TestMain(m *testing.M) {
	// === Set up Test Database ===
	defaultConn := baseConnStr + " dbname=postgres"
	db, err := sql.Open("pgx", defaultConn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to postgres: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to ping postgres: %v\n", err)
		os.Exit(1)
	}

	_, _ = db.Exec("DROP DATABASE IF EXISTS " + testDBName)
	_, err = db.Exec("CREATE DATABASE " + testDBName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test database: %v\n", err)
		os.Exit(1)
	}

	// Wait briefly to ensure the new test database is ready.
	time.Sleep(1 * time.Second)

	// Connect to the newly created test database.
	testDBConnStr := baseConnStr + " dbname=" + testDBName
	testDB, err := sql.Open("pgx", testDBConnStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to test database: %v\n", err)
		os.Exit(1)
	}
	defer testDB.Close()

	// Create the test schema.
	_, err = testDB.Exec("CREATE SCHEMA IF NOT EXISTS " + testSchema)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create test schema: %v\n", err)
		os.Exit(1)
	}

	// === Build CLI Binary ===
	binaryPath := filepath.Join(os.TempDir(), "gostgrator-pg-integration")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build CLI binary: %v\n", err)
		os.Exit(1)
	}
	cliBinary = binaryPath

	// === Run Tests ===
	code := m.Run()

	// === Tear Down Test Database ===
	db2, err := sql.Open("pgx", defaultConn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to reconnect for cleanup: %v\n", err)
		os.Exit(1)
	}
	defer db2.Close()

	_, err = db2.Exec(fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='%s'", testDBName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not terminate connections: %v\n", err)
	}
	_, err = db2.Exec("DROP DATABASE IF EXISTS " + testDBName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to drop test database: %v\n", err)
	}

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

// makeTestConnURL constructs a URL-style DSN, for example:
//   postgres://postgres@localhost:5432/gostgrator_cli_test?sslmode=disable&search_path=gostgrator_schema
func makeTestConnURL() string {
	testConnURL := fmt.Sprintf("postgres://postgres@localhost:5432/%s?sslmode=disable", testDBName)
	return testConnURL + "&search_path=" + testSchema
}

// tableExists returns true if the given table name exists in the current search_path.
func tableExists(db *sql.DB, table string) (bool, error) {
	var exists bool
	q := `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name=$1)`
	if err := db.QueryRow(q, table).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// -----------------------------------------------------------------------------
// Core CLI command integration tests (existing behaviour)
// -----------------------------------------------------------------------------

// TestCLIMigrate tests the "migrate" command.
func TestCLIMigrate(t *testing.T) {
	connArg := makeTestConnURL()
	env := fmt.Sprintf("DATABASE_URL=%s", connArg)
	args := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"migrate", "max",
	}
	out, err := helperRun(args, env)
	if err != nil {
		t.Fatalf("CLI migrate command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Starting migration") {
		t.Errorf("expected migration start message, got:\n%s", out)
	}
}

// TestCLIDown tests the "down" command.
func TestCLIDown(t *testing.T) {
	connArg := makeTestConnURL()
	env := fmt.Sprintf("DATABASE_URL=%s", connArg)
	args := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"down", "1",
	}
	out, err := helperRun(args, env)
	if err != nil {
		t.Fatalf("CLI down command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Rolling back") {
		t.Errorf("expected rollback message, got:\n%s", out)
	}
}

// TestCLIMigrateDownToZero tests migrating down to version 0 using the migrate command.
// This ensures that the CLI supports migration in both directions.
func TestCLIMigrateDownToZero(t *testing.T) {
	connArg := makeTestConnURL()
	env := fmt.Sprintf("DATABASE_URL=%s", connArg)

	// First, migrate up to max (assumed max version is 6).
	migrateArgs := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"migrate", "max",
	}
	out, err := helperRun(migrateArgs, env)
	if err != nil {
		t.Fatalf("CLI migrate command (to max) failed: %v; output: %s", err, out)
	}

	// Now, migrate down to version 0.
	downTarget := "0"
	downArgs := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"migrate", downTarget,
	}
	out, err = helperRun(downArgs, env)
	if err != nil {
		t.Fatalf("CLI migrate command (down to 0) failed: %v; output: %s", err, out)
	}

	// Run list command to confirm current version is 0.
	listArgs := []string{
		"-conn", connArg,
		"-migration-pattern", testMigrationsPath,
		"list",
	}
	out, err = helperRun(listArgs, env)
	if err != nil {
		t.Fatalf("CLI list command (after migrate down to 0) failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version: 0") {
		t.Errorf("expected current migration version 0 after migrating down, got:\n%s", out)
	}
}

// TestCLIDropSchema tests the "drop-schema" command.
func TestCLIDropSchema(t *testing.T) {
	connArg := makeTestConnURL()
	env := fmt.Sprintf("DATABASE_URL=%s", connArg)
	args := []string{
		"-conn", connArg,
		"drop-schema",
	}
	out, err := helperRun(args, env)
	if err != nil {
		t.Fatalf("CLI drop-schema command failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Dropping schema table") {
		t.Errorf("expected drop schema message, got:\n%s", out)
	}
}

// TestCLINew tests the "new" command which creates migration files.
// For "new", we use a dummy connection since a live DB is not needed.
func TestCLINew(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli_new_integration")
	if err != nil {
		t.Fatalf("failed to create temp migration directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := map[string]interface{}{
		"MigrationPattern": filepath.Join(tmpDir, "*.sql"),
		"Driver":           "pg",
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
		t.Fatalf("CLI new command failed: %v; output: %s", err, out)
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

// TestFlagOrderingSafe verifies that the safeguard against flags placed after positional arguments works.
func TestFlagOrderingSafe(t *testing.T) {
	out, _ := helperRun([]string{"migrate", "-conn", "dummy"})
	expected := "Error: Flags must be specified before the command. Please reorder your arguments."
	if !strings.Contains(out, expected) {
		t.Errorf("expected flag ordering error message, got:\n%s", out)
	}
}

// -----------------------------------------------------------------------------
// New precedence & schema‑table override integration tests
// -----------------------------------------------------------------------------

// TestConnPrecedence_ConfigUsed checks that conn from config file works when flag/env absent.
func TestConnPrecedence_ConfigUsed(t *testing.T) {
	connGood := makeTestConnURL()

	tmpDir, _ := os.MkdirTemp("", "precedence_cfg")
	defer os.RemoveAll(tmpDir)

	cfg := map[string]interface{}{
		"conn":             connGood,
		"MigrationPattern": testMigrationsPath,
	}
	cfgPath := filepath.Join(tmpDir, "config.json")
	b, _ := os.Create(cfgPath)
	json.NewEncoder(b).Encode(cfg)
	b.Close()

	out, err := helperRun([]string{"-config", cfgPath, "list"}, "DATABASE_URL=")
	if err != nil {
		t.Fatalf("CLI list with config conn failed: %v; out: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version") {
		t.Errorf("expected successful DB access via config conn; got:\n%s", out)
	}
}

// TestConnPrecedence_EnvWins checks env beats config.
func TestConnPrecedence_EnvWins(t *testing.T) {
	connGood := makeTestConnURL()
	connBad := "postgres://invalid_host/db?sslmode=disable"

	tmpDir, _ := os.MkdirTemp("", "precedence_env")
	defer os.RemoveAll(tmpDir)

	cfg := map[string]interface{}{
		"conn":             connBad,
		"MigrationPattern": testMigrationsPath,
	}
	cfgPath := filepath.Join(tmpDir, "config.json")
	b, _ := os.Create(cfgPath)
	json.NewEncoder(b).Encode(cfg)
	b.Close()

	out, err := helperRun([]string{"-config", cfgPath, "list"},
		fmt.Sprintf("DATABASE_URL=%s", connGood))
	if err != nil {
		t.Fatalf("CLI list with env conn failed: %v; out: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version") {
		t.Errorf("expected env conn to override bad config conn; got:\n%s", out)
	}
}

// TestConnPrecedence_FlagWins checks flag beats env+config.
func TestConnPrecedence_FlagWins(t *testing.T) {
	connGood := makeTestConnURL()
	connBad := "postgres://invalid_host/db?sslmode=disable"

	tmpDir, _ := os.MkdirTemp("", "precedence_flag")
	defer os.RemoveAll(tmpDir)

	cfg := map[string]interface{}{
		"conn":             connBad,
		"MigrationPattern": testMigrationsPath,
	}
	cfgPath := filepath.Join(tmpDir, "config.json")
	b, _ := os.Create(cfgPath)
	json.NewEncoder(b).Encode(cfg)
	b.Close()

	out, err := helperRun(
		[]string{"-conn", connGood, "-config", cfgPath, "list"},
		fmt.Sprintf("DATABASE_URL=%s", connBad))
	if err != nil {
		t.Fatalf("CLI list with flag conn failed: %v; out: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version") {
		t.Errorf("expected flag conn to win; got:\n%s", out)
	}
}

// TestSchemaTableFlagOverridesConfig verifies that -schema-table overrides the value in config.
func TestSchemaTableFlagOverridesConfig(t *testing.T) {
	connArg := makeTestConnURL()

	// Use distinct table names so we can detect which one was created.
	configTable := "schemaversion_cfg"
	flagTable := "schemaversion_flag"

	tmpDir, _ := os.MkdirTemp("", "schema_override")
	defer os.RemoveAll(tmpDir)

	cfg := map[string]interface{}{
		"SchemaTable":      configTable,
		"MigrationPattern": testMigrationsPath,
		"conn":             connArg,
	}
	cfgPath := filepath.Join(tmpDir, "config.json")
	cf, _ := os.Create(cfgPath)
	json.NewEncoder(cf).Encode(cfg)
	cf.Close()

	// Run migrate with overriding flag.
	_, err := helperRun([]string{
		"-config", cfgPath,
		"-schema-table", flagTable,
		"migrate", "max",
	})
	if err != nil {
		t.Fatalf("CLI migrate for schema override failed: %v", err)
	}

	// Connect to DB directly to verify which schemaversion table exists.
	db, err := sql.Open("pgx", connArg)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer db.Close()

	okFlag, err := tableExists(db, flagTable)
	if err != nil {
		t.Fatalf("tableExists: %v", err)
	}
	okConfig, err := tableExists(db, configTable)
	if err != nil {
		t.Fatalf("tableExists: %v", err)
	}
	if !okFlag || okConfig {
		t.Errorf("expected %s to exist and %s not to; got flagExists=%v configExists=%v",
			flagTable, configTable, okFlag, okConfig)
	}
}
