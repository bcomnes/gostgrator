// main_test.go
package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain triggers helper process mode.
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
