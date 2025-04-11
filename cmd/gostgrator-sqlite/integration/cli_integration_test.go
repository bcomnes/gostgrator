// cli_integration_test.go
package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var cliBinary string

// Global variables for SQLite testing.
var (
	// We use a temporary file for the SQLite DB.
	testDBFile = filepath.Join(os.TempDir(), "gostgrator_sqlite_test.db")
	// testMigrationsPath: relative path from the integration test package to the test migration files.
	testMigrationsPath = "../../../pkg/gostgrator/testdata/migrations/*.sql"
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

// TestCLIListChain tests the "list" command by chaining operations:
// 1. Reset migrations down to 0 using "migrate 0".
// 2. Run list before any migrations; expect current version 0.
// 3. Run migrate (target max) and then list: expect current version equals max version (e.g., 6).
// 4. Run down (roll back 1) and then list: expect current version decreases (e.g., 5).
// Also prints the initial list output for debugging.
func TestCLIListChain(t *testing.T) {
	connArg := makeTestConnURL()

	// Reset: migrate down to 0 to clear any previous state.
	resetArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "migrate", "0"}
	_, err := helperRun(resetArgs)
	if err != nil {
		t.Fatalf("SQLite CLI reset (migrate 0) failed: %v", err)
	}

	listArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "list"}

	// Step 1: List after reset; expect current version to be 0.
	out, err := helperRun(listArgs)
	if err != nil {
		t.Fatalf("SQLite CLI list command (initial) failed: %v; output: %s", err, out)
	}
	fmt.Println("DEBUG: Initial list output:")
	fmt.Println(out)
	if !strings.Contains(out, "Current database migration version: 0") {
		t.Errorf("expected current migration version 0 initially, got:\n%s", out)
	}

	// Step 2: Migrate to max (assumed max version is 6), then list.
	migrateArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "migrate", "max"}
	out, err = helperRun(migrateArgs)
	if err != nil {
		t.Fatalf("SQLite CLI migrate command failed: %v; output: %s", err, out)
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

	// Step 3: Run down to roll back one migration, then list again. Now current version should be 5.
	downArgs := []string{"-conn", connArg, "-migration-pattern", testMigrationsPath, "down", "1"}
	out, err = helperRun(downArgs)
	if err != nil {
		t.Fatalf("SQLite CLI down command failed: %v; output: %s", err, out)
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
