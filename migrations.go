package gostgrator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Migration represents a single migration file.
type Migration struct {
	// Version of the migration.
	Version int

	// Action, e.g., "do" or "undo".
	Action string

	// Filename is the path to the migration file.
	Filename string

	// Name is an optional descriptive name of the migration.
	Name string

	// Md5 is the MD5 checksum of the migration file.
	Md5 string
}

// getSQL reads the migration file's content.
func (m *Migration) getSQL() (string, error) {
	data, err := os.ReadFile(m.Filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// sortMigrationsAsc sorts migrations in ascending order based on version.
func sortMigrationsAsc(migs []Migration) {
	sort.Slice(migs, func(i, j int) bool {
		return migs[i].Version < migs[j].Version
	})
}

// sortMigrationsDesc sorts migrations in descending order based on version.
func sortMigrationsDesc(migs []Migration) {
	sort.Slice(migs, func(i, j int) bool {
		return migs[i].Version > migs[j].Version
	})
}

// convertLineEnding converts all newline variations in content to the target style.
func convertLineEnding(content, lineEnding string) (string, error) {
	var target string
	switch lineEnding {
	case "LF":
		target = "\n"
	case "CR":
		target = "\r"
	case "CRLF":
		target = "\r\n"
	default:
		return "", fmt.Errorf("newline must be one of: LF, CR, CRLF")
	}
	re := regexp.MustCompile(`\r\n|\r|\n`)
	return re.ReplaceAllString(content, target), nil
}

// checksum computes the MD5 checksum of the content after converting line endings if set.
func checksum(content, lineEnding string) (string, error) {
	if lineEnding != "" {
		var err error
		content, err = convertLineEnding(content, lineEnding)
		if err != nil {
			return "", err
		}
	}
	sum := md5.Sum([]byte(content))
	return hex.EncodeToString(sum[:]), nil
}

// fileChecksum reads a file and returns its MD5 checksum.
func fileChecksum(filename, lineEnding string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return checksum(string(data), lineEnding)
}

// getMigrations scans for migration files matching the pattern and loads them.
func getMigrations(cfg Config) ([]Migration, error) {
	files, err := filepath.Glob(cfg.MigrationPattern)
	if err != nil {
		return nil, err
	}
	var migrations []Migration
	migrationKeys := make(map[string]struct{})
	for _, file := range files {
		if filepath.Ext(file) != ".sql" {
			continue
		}
		base := filepath.Base(file)
		ext := filepath.Ext(base)
		baseNoExt := strings.TrimSuffix(base, ext)
		parts := strings.Split(baseNoExt, ".")
		if len(parts) < 2 {
			// Skip files that do not match version.action[.name]
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		action := parts[1]
		name := ""
		if len(parts) > 2 {
			name = strings.Join(parts[2:], ".")
		}
		md5sum, err := fileChecksum(file, cfg.Newline)
		if err != nil {
			return nil, err
		}
		mig := Migration{
			Version:  version,
			Action:   action,
			Filename: file,
			Name:     name,
			Md5:      md5sum,
		}
		key := fmt.Sprintf("%d:%s", mig.Version, mig.Action)
		if _, exists := migrationKeys[key]; exists {
			return nil, fmt.Errorf("duplicate migration for version %d and action %s", mig.Version, mig.Action)
		}
		migrationKeys[key] = struct{}{}
		migrations = append(migrations, mig)
	}
	return migrations, nil
}
