package gostgrator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CreateMigration creates a new pair of migration files (do/undo).
// description: a human-readable description that will be kebab-cased for the filename.
// mode: "int" for integer increment (default) or "timestamp" to use the Unix timestamp.
func CreateMigration(cfg Config, description string, mode string) error {
	// Determine the migration folder from the migration pattern.
	migFolder := filepath.Dir(cfg.MigrationPattern)

	// Get the next migration number as a string.
	var nextNumber string
	files, err := filepath.Glob(cfg.MigrationPattern)
	if err != nil {
		return fmt.Errorf("failed to scan migration files: %w", err)
	}
	if strings.ToLower(mode) == "timestamp" {
		nextNumber = strconv.FormatInt(time.Now().Unix(), 10)
	} else {
		// Default: integer mode with triple zero-padding.
		max := 0
		for _, file := range files {
			base := filepath.Base(file)
			parts := strings.Split(base, ".")
			if len(parts) < 2 {
				continue
			}
			// Parse without padding.
			num, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			if num > max {
				max = num
			}
		}
		// Use triple zero-padded integer.
		nextNumber = fmt.Sprintf("%03d", max+1)
	}

	// Convert the description into kebab-case.
	kebabDesc := kebabCase(description)

	// Build file names.
	doFilename := fmt.Sprintf("%s.do.%s.sql", nextNumber, kebabDesc)
	undoFilename := fmt.Sprintf("%s.undo.%s.sql", nextNumber, kebabDesc)

	// Build full file paths.
	doFilePath := filepath.Join(migFolder, doFilename)
	undoFilePath := filepath.Join(migFolder, undoFilename)

	// Write empty template content.
	doContent := []byte("-- Write your migration SQL here\n")
	undoContent := []byte("-- Write your rollback SQL here\n")

	if err := os.WriteFile(doFilePath, doContent, 0644); err != nil {
		return fmt.Errorf("failed to create migration file %s: %w", doFilePath, err)
	}
	if err := os.WriteFile(undoFilePath, undoContent, 0644); err != nil {
		return fmt.Errorf("failed to create migration file %s: %w", undoFilePath, err)
	}

	return nil
}

// kebabCase converts a string to kebab-case.
func kebabCase(s string) string {
	// Lowercase and trim spaces.
	s = strings.ToLower(strings.TrimSpace(s))
	// Replace any non-alphanumeric sequence with a single hyphen.
	re := regexp.MustCompile("[^a-z0-9]+")
	s = re.ReplaceAllString(s, "-")
	// Trim any hyphens from the beginning or end.
	return strings.Trim(s, "-")
}

// (Optional) If you prefer to expose this functionality as a method on Gostgrator,
// you can add the following method.
func (g *Gostgrator) CreateMigration(description, mode string) error {
	return CreateMigration(g.cfg, description, mode)
}
