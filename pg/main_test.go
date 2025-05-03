package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain triggers our helper process mode. When the environment
// variable GO_HELPER_PROCESS is set, main() is called (simulating our CLI).
func TestMain(m *testing.M) {
	if os.Getenv("GO_HELPER_PROCESS") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runCLI runs the current test binary as a helper process running the CLI.
// It passes along the provided arguments and any extra environment variables.
func runCLI(args []string, extraEnv ...string) (string, error) {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_HELPER_PROCESS=1")
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestCLIHelp checks that -help prints the usage info.
func TestCLIHelp(t *testing.T) {
	out, _ := runCLI([]string{"-help"})
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected help usage info, got:\n%s", out)
	}
}

// TestCLIVersion checks that -version prints the version string.
func TestCLIVersion(t *testing.T) {
	out, _ := runCLI([]string{"-version"})
	if !strings.Contains(out, "gostgrator-pg version:") {
		t.Errorf("expected version info, got:\n%s", out)
	}
}

// TestCLINoCommand ensures that running the CLI with no command shows an error.
func TestCLINoCommand(t *testing.T) {
	out, _ := runCLI([]string{})
	if !strings.Contains(out, "Error: no command provided.") {
		t.Errorf("expected error for missing command, got:\n%s", out)
	}
}

// TestCLIUnknownCommand checks that an unknown command produces an error.
func TestCLIUnknownCommand(t *testing.T) {
	out, _ := runCLI([]string{"foobar"})
	if !strings.Contains(out, "Unknown command: foobar") {
		t.Errorf("expected unknown command error, got:\n%s", out)
	}
}

// TestCLIMigrateMissingConn verifies that when running 'migrate' with no connection,
// an error message is printed about the missing connection URL.
func TestCLIMigrateMissingConn(t *testing.T) {
	// Override DATABASE_URL to an empty string to simulate a missing connection.
	out, _ := runCLI([]string{"migrate", "max"}, "DATABASE_URL=")
	if !strings.Contains(out, "Error: connection URL must be provided") {
		t.Errorf("expected connection URL missing error, got:\n%s", out)
	}
}

// TestCLIDownInvalidSteps verifies that if a non-numeric rollback step is given,
// an error is printed.
func TestCLIDownInvalidSteps(t *testing.T) {
	// Global flags come first.
	out, _ := runCLI([]string{"-conn", "dummy", "down", "abc"})
	if !strings.Contains(out, "Invalid rollback steps: abc") {
		t.Errorf("expected rollback steps error, got:\n%s", out)
	}
}

// TestCLINewMissingDescription checks that using the new command without a description prints an error.
func TestCLINewMissingDescription(t *testing.T) {
	// Pass flags before the positional command. No description is provided.
	out, _ := runCLI([]string{"-conn", "dummy", "new"})
	if !strings.Contains(out, "Error: a description is required for the new command.") {
		t.Errorf("expected missing description error, got:\n%s", out)
	}
}

// TestCLIConfigLoadError checks that if a config file does not exist, an error is printed.
func TestCLIConfigLoadError(t *testing.T) {
	// Place the flags first.
	out, _ := runCLI([]string{"-conn", "dummy", "-config", "nonexistent.json", "migrate"})
	if !strings.Contains(out, "Error loading config file:") {
		t.Errorf("expected config file loading error, got:\n%s", out)
	}
}

// TestCLINewSuccess creates a temporary config and migration directory,
// then runs the new command. It confirms that a success message is printed
// and that two migration files (do and undo) have been created.
func TestCLINewSuccess(t *testing.T) {
	// Create a temporary directory for migration files.
	tmpDir, err := os.MkdirTemp("", "cli_new_success")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a temporary config file with MigrationPattern set to our temporary directory.
	cfg := map[string]interface{}{
		"MigrationPattern": filepath.Join(tmpDir, "*.sql"),
		"Driver":           "pg",
		"SchemaTable":      "schemaversion",
		"ValidateChecksums": true,
	}
	cfgFile, err := os.CreateTemp("", "cli_config_*.json")
	if err != nil {
		t.Fatalf("failed to create temp config file: %v", err)
	}
	encoder := json.NewEncoder(cfgFile)
	if err := encoder.Encode(cfg); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	cfgFile.Close()
	defer os.Remove(cfgFile.Name())

	description := "Create test table"
	// Flags come first before the positional command and description.
	out, _ := runCLI([]string{"-conn", "dummy", "-config", cfgFile.Name(), "-mode", "int", "new", description})
	if !strings.Contains(out, "New migration created successfully.") {
		t.Errorf("expected new migration success message, got:\n%s", out)
	}

	// Check that two migration files exist in the temporary directory.
	matches, err := filepath.Glob(filepath.Join(tmpDir, "*.sql"))
	if err != nil {
		t.Fatalf("failed to glob migration files: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("expected 2 migration files, got %d", len(matches))
	}
}

// TestCLIDropSchemaMissingConn checks that running drop-schema without a connection URL
// prints an error.
func TestCLIDropSchemaMissingConn(t *testing.T) {
	// Override DATABASE_URL to an empty string.
	out, _ := runCLI([]string{"drop-schema"}, "DATABASE_URL=")
	if !strings.Contains(out, "Error: connection URL must be provided") {
		t.Errorf("expected connection URL error for drop-schema, got:\n%s", out)
	}
}

// TestFlagOrderingSafe verifies that the safeguard against flags placed after positional arguments works.
func TestFlagOrderingSafe(t *testing.T) {
	// Intentionally pass a flag after the positional command.
	// For example, "migrate" as the command, then "-conn" as a positional argument.
	out, _ := runCLI([]string{"migrate", "-conn", "dummy"})
	expected := "Error: Flags must be specified before the command. Please reorder your arguments."
	if !strings.Contains(out, expected) {
		t.Errorf("expected flag ordering error message, got:\n%s", out)
	}
}
