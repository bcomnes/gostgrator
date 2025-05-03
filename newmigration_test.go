package gostgrator

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestCreateMigrationIntMode verifies that in integer mode the new migration files
// are created with the correct triple zero-padded naming convention and contain the expected template content.
func TestCreateMigrationIntMode(t *testing.T) {
	// Create a temporary directory for migrations.
	tmpDir, err := os.MkdirTemp("", "migrations_test_int")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a configuration with the migration pattern pointing to our temp dir.
	cfg := Config{
		MigrationPattern: filepath.Join(tmpDir, "*.sql"),
	}

	description := "Add new table"
	mode := "int"

	// Call CreateMigration.
	if err := CreateMigration(cfg, description, mode); err != nil {
		t.Fatalf("CreateMigration failed: %v", err)
	}

	// Expect triple zero padded integer mode: if no existing migrations, it should start with "001".
	doExpected := filepath.Join(tmpDir, "001.do.add-new-table.sql")
	undoExpected := filepath.Join(tmpDir, "001.undo.add-new-table.sql")

	// Check that both files exist.
	if _, err := os.Stat(doExpected); os.IsNotExist(err) {
		t.Errorf("expected do file %s to exist", doExpected)
	}
	if _, err := os.Stat(undoExpected); os.IsNotExist(err) {
		t.Errorf("expected undo file %s to exist", undoExpected)
	}

	// Check file contents.
	doContent, err := os.ReadFile(doExpected)
	if err != nil {
		t.Errorf("failed to read do file: %v", err)
	}
	if !strings.Contains(string(doContent), "Write your migration SQL here") {
		t.Errorf("do file content not as expected: %s", string(doContent))
	}

	undoContent, err := os.ReadFile(undoExpected)
	if err != nil {
		t.Errorf("failed to read undo file: %v", err)
	}
	if !strings.Contains(string(undoContent), "Write your rollback SQL here") {
		t.Errorf("undo file content not as expected: %s", string(undoContent))
	}
}

// TestCreateMigrationTimestampMode verifies that in timestamp mode the new migration files
// are created with a Unix timestamp as the prefix and contain the expected template content.
func TestCreateMigrationTimestampMode(t *testing.T) {
	// Create a temporary directory for migrations.
	tmpDir, err := os.MkdirTemp("", "migrations_test_ts")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a configuration with the migration pattern pointing to our temp dir.
	cfg := Config{
		MigrationPattern: filepath.Join(tmpDir, "*.sql"),
	}

	description := "Fix bug"
	mode := "timestamp"

	// Call CreateMigration.
	if err := CreateMigration(cfg, description, mode); err != nil {
		t.Fatalf("CreateMigration failed: %v", err)
	}

	// List all files in the temporary directory.
	files, err := filepath.Glob(filepath.Join(tmpDir, "*.sql"))
	if err != nil {
		t.Fatalf("failed to glob migration files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 migration files, got %d", len(files))
	}

	// Identify the do and undo files.
	var doFile, undoFile string
	for _, f := range files {
		if strings.Contains(f, ".do.") {
			doFile = f
		} else if strings.Contains(f, ".undo.") {
			undoFile = f
		}
	}
	if doFile == "" || undoFile == "" {
		t.Fatalf("did not find both do and undo migration files")
	}

	// For timestamp mode, check that the first part of the file name is a valid timestamp.
	baseDo := filepath.Base(doFile)
	parts := strings.Split(baseDo, ".")
	if len(parts) < 3 {
		t.Fatalf("unexpected file name format: %s", baseDo)
	}
	timestampStr := parts[0]
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		t.Errorf("expected timestamp number, got %s", timestampStr)
	}
	// Ensure the timestamp is within a reasonable range (e.g. within the last minute).
	if time.Since(time.Unix(timestamp, 0)) > time.Minute {
		t.Errorf("timestamp %d seems too old", timestamp)
	}

	// Check file contents.
	doContent, err := os.ReadFile(doFile)
	if err != nil {
		t.Errorf("failed to read do file: %v", err)
	}
	if !strings.Contains(string(doContent), "Write your migration SQL here") {
		t.Errorf("do file content not as expected: %s", string(doContent))
	}

	undoContent, err := os.ReadFile(undoFile)
	if err != nil {
		t.Errorf("failed to read undo file: %v", err)
	}
	if !strings.Contains(string(undoContent), "Write your rollback SQL here") {
		t.Errorf("undo file content not as expected: %s", string(undoContent))
	}
}
