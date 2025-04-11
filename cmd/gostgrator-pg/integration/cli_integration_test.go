// cli_integration_test.go
package integration

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
	testDBName         = "gostgrator_cli_test"
	testSchema         = "gostgrator_schema"
	// Base connection string for DSN-based connections used in TestMain.
	baseConnStr        = "host=localhost port=5432 user=postgres sslmode=disable"
	// testMigrationsPath: relative path from the integration test package to the test migration files.
	testMigrationsPath = "../../../pkg/gostgrator/testdata/migrations/*.sql"
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

// TestCLIListChain tests the "list" command by chaining operations:
// 1. Run list before any migrations; expect current version 0.
// 2. Run migrate (target max) and then list: expect current version equals max version (e.g., 6).
// 3. Run down (roll back 1) and then list: expect current version decreases (e.g., 5).
// Also prints the initial list output for debugging.
func TestCLIListChain(t *testing.T) {
	connArg := makeTestConnURL()
	env := fmt.Sprintf("DATABASE_URL=%s", connArg)
	listArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "list"}

	// Step 1: List before migrations.
	out, err := helperRun(listArgs, env)
	if err != nil {
		t.Fatalf("CLI list command (initial) failed: %v; output: %s", err, out)
	}
	// Debug print of the initial list.
	//fmt.Println("DEBUG: Initial list output:")
	//fmt.Println(out)
	if !strings.Contains(out, "Current database migration version: 0") {
		t.Errorf("expected current migration version 0 initially, got:\n%s", out)
	}

	// Step 2: Migrate to max (assumed max version is 6), then list.
	migrateArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "migrate", "max"}
	out, err = helperRun(migrateArgs, env)
	//fmt.Println("DEBUG: Initial list output:")
	//fmt.Println(out)
	if err != nil {
		t.Fatalf("CLI migrate command failed: %v; output: %s", err, out)
	}
	out, err = helperRun(listArgs, env)
	//fmt.Println("DEBUG: Initial list output:")
	//fmt.Println(out)
	if err != nil {
		t.Fatalf("CLI list command (after migrate) failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version: 6") {
		t.Errorf("expected current migration version 6 after migrate, got:\n%s", out)
	}
	if !strings.Contains(out, "Version 6:") || !strings.Contains(out, "<== current") {
		t.Errorf("expected migration version 6 to be annotated as current, got:\n%s", out)
	}

	// Step 3: Run down to roll back one migration, then list again. Now current version should be 5.
	downArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "down", "1"}
	out, err = helperRun(downArgs, env)
	if err != nil {
		t.Fatalf("CLI down command failed: %v; output: %s", err, out)
	}
	out, err = helperRun(listArgs, env)
	if err != nil {
		t.Fatalf("CLI list command (after down) failed: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Current database migration version: 5") {
		t.Errorf("expected current migration version 5 after down, got:\n%s", out)
	}
	if !strings.Contains(out, "Version 5:") || !strings.Contains(out, "<== current") {
		t.Errorf("expected migration version 5 to be annotated as current, got:\n%s", out)
	}
}
