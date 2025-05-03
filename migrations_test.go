package gostgrator

import (
	"testing"
)

// TestConvertLineEnding_LF verifies that converting to LF produces the expected result.
func TestConvertLineEnding_LF(t *testing.T) {
	// Original content using LF as the newline.
	content := "line one\nline two\nlinethree\nlinefour"
	expected := "line one\nline two\nlinethree\nlinefour"

	got, err := convertLineEnding(content, "LF")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
}

// TestConvertLineEnding_CR verifies that converting to CR produces the expected result.
func TestConvertLineEnding_CR(t *testing.T) {
	// Original content using LF newlines.
	content := "line one\nline two\nlinethree\nlinefour"
	expected := "line one\rline two\rlinethree\rlinefour"

	got, err := convertLineEnding(content, "CR")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
}

// TestConvertLineEnding_CRLF verifies that converting to CRLF produces the expected result.
func TestConvertLineEnding_CRLF(t *testing.T) {
	// Original content using LF newlines.
	content := "line one\nline two\nlinethree\nlinefour"
	expected := "line one\r\nline two\r\nlinethree\r\nlinefour"

	got, err := convertLineEnding(content, "CRLF")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
}

// TestConvertLineEnding_Invalid verifies that an invalid newline type returns an error.
func TestConvertLineEnding_Invalid(t *testing.T) {
	_, err := convertLineEnding("line one\nline two", "INVALID")
	if err == nil {
		t.Errorf("Expected an error for invalid newline type, got nil")
	}
}
