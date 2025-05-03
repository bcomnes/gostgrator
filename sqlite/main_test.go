// main_test.go
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Helper process setup
// -----------------------------------------------------------------------------

// TestMain triggers helper process mode when GO_HELPER_PROCESS is set.
func TestMain(m *testing.M) {
	if os.Getenv("GO_HELPER_PROCESS") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runCLI runs the current test binary as a helper process running the CLI.
func runCLI(args []string, extraEnv ...string) (string, error) {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "GO_HELPER_PROCESS=1")
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// makeTempConfig writes a tiny JSON config with a "conn" value and returns
// the file path and a cleanup func.
func makeTempConfig(conn string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "sqlite_cli_cfg")
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "cfg.json")
	f, err := os.Create(path)
	if err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}
	json.NewEncoder(f).Encode(map[string]any{
		"conn": conn,
	})
	f.Close()
	cleanup := func() { os.RemoveAll(dir) }
	return path, cleanup, nil
}

// -----------------------------------------------------------------------------
// Baseline CLI behaviour tests (unchanged)
// -----------------------------------------------------------------------------

// TestCLIHelp checks that -help prints usage info.
func TestCLIHelp(t *testing.T) {
	out, _ := runCLI([]string{"-help"})
	if !strings.Contains(out, "Usage:") {
		t.Errorf("expected help usage info, got:\n%s", out)
	}
}

// TestCLIVersion checks that -version prints version string.
func TestCLIVersion(t *testing.T) {
	out, _ := runCLI([]string{"-version"})
	if !strings.Contains(out, "gostgrator-sqlite version:") {
		t.Errorf("expected version info, got:\n%s", out)
	}
}

// TestCLINoCommand ensures running with no command shows an error.
func TestCLINoCommand(t *testing.T) {
	out, _ := runCLI([]string{})
	if !strings.Contains(out, "Error: no command provided.") {
		t.Errorf("expected no command error, got:\n%s", out)
	}
}

// TestCLIUnknownCommand checks that an unknown command produces an error.
func TestCLIUnknownCommand(t *testing.T) {
	out, _ := runCLI([]string{"foobar"})
	if !strings.Contains(out, "Unknown command: foobar") {
		t.Errorf("expected unknown command error, got:\n%s", out)
	}
}

// TestFlagOrderingSafe verifies the safeguard against flags after positional arguments.
func TestFlagOrderingSafe(t *testing.T) {
	out, _ := runCLI([]string{"migrate", "-conn", "dummy"})
	expected := "Error: Flags must be specified before the command. Please reorder your arguments."
	if !strings.Contains(out, expected) {
		t.Errorf("expected flag ordering error, got:\n%s", out)
	}
}

// TestCLIConfigLoadError checks that a missing config file produces an error.
func TestCLIConfigLoadError(t *testing.T) {
	out, _ := runCLI([]string{"-conn", "dummy", "-config", "nonexistent.json", "migrate"})
	if !strings.Contains(out, "Error loading config file:") {
		t.Errorf("expected config load error, got:\n%s", out)
	}
}

// -----------------------------------------------------------------------------
// New connection‑precedence tests
// -----------------------------------------------------------------------------

// fileExists is a tiny helper to assert DB file creation.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TestConnPrecedence_FlagWins ensures -conn beats env and config.
func TestConnPrecedence_FlagWins(t *testing.T) {
	tmpDir := t.TempDir()
	flagDB := filepath.Join(tmpDir, "flag.db")
	envDB := filepath.Join(tmpDir, "env.db")
	cfgDB := filepath.Join(tmpDir, "cfg.db")

	cfgPath, clean, err := makeTempConfig(cfgDB)
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	defer clean()

	_, err = runCLI(
		[]string{"-conn", flagDB, "-config", cfgPath, "list"},
		"SQLITE_URL="+envDB,
	)
	if err != nil {
		t.Fatalf("CLI run: %v", err)
	}

	if !fileExists(flagDB) || fileExists(envDB) || fileExists(cfgDB) {
		t.Errorf("expected only flag DB to be created (precedence failed)")
	}
}

// TestConnPrecedence_EnvWins ensures env beats config.
func TestConnPrecedence_EnvWins(t *testing.T) {
	tmpDir := t.TempDir()
	envDB := filepath.Join(tmpDir, "env.db")
	cfgDB := filepath.Join(tmpDir, "cfg.db")

	cfgPath, clean, err := makeTempConfig(cfgDB)
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	defer clean()

	_, err = runCLI([]string{"-config", cfgPath, "list"}, "SQLITE_URL="+envDB)
	if err != nil {
		t.Fatalf("CLI run: %v", err)
	}

	if !fileExists(envDB) || fileExists(cfgDB) {
		t.Errorf("expected env DB to be used over config DB")
	}
}

// TestConnPrecedence_ConfigUsed ensures config is used when flag/env absent.
func TestConnPrecedence_ConfigUsed(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDB := filepath.Join(tmpDir, "cfg.db")

	cfgPath, clean, err := makeTempConfig(cfgDB)
	if err != nil {
		t.Fatalf("cfg: %v", err)
	}
	defer clean()

	_, err = runCLI([]string{"-config", cfgPath, "list"}, "SQLITE_URL=")
	if err != nil {
		t.Fatalf("CLI run: %v", err)
	}

	if !fileExists(cfgDB) {
		t.Errorf("expected config DB to be created/used")
	}
}

// TestConnPrecedence_MissingEverywhere ensures error when no connection info.
func TestConnPrecedence_MissingEverywhere(t *testing.T) {
	out, _ := runCLI([]string{"list"}, "SQLITE_URL=")
	if !strings.Contains(out, "connection URL must be provided") {
		t.Errorf("expected missing conn error, got:\n%s", out)
	}
}
