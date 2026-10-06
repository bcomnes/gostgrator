package gostgrator

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
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

	// Filename is the OS path, or the path relative to the source filesystem.
	Filename string

	// Name is an optional descriptive name of the migration.
	Name string

	// Md5 is the MD5 checksum of the migration file.
	Md5 string

	filesystem fs.FS
}

// getSQL reads the migration file's content.
func (m *Migration) getSQL() (string, error) {
	data, err := readMigrationFile(m.filesystem, m.Filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readMigrationFile(filesystem fs.FS, filename string) ([]byte, error) {
	if filesystem != nil {
		return fs.ReadFile(filesystem, filename)
	}
	return os.ReadFile(filename)
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
	filesystem, pattern, err := migrationSource(cfg)
	if err != nil {
		return nil, err
	}
	var files []string
	if filesystem != nil {
		files, err = fs.Glob(filesystem, pattern)
	} else {
		files, err = filepath.Glob(pattern)
	}
	if err != nil {
		return nil, err
	}
	var migrations []Migration
	migrationKeys := make(map[string]struct{})
	for _, file := range files {
		base := filepath.Base(file)
		if filesystem != nil {
			base = path.Base(file)
		}
		ext := path.Ext(base)
		if ext != ".sql" {
			continue
		}
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
		data, err := readMigrationFile(filesystem, file)
		if err != nil {
			return nil, err
		}
		md5sum, err := checksum(string(data), cfg.Newline)
		if err != nil {
			return nil, err
		}
		mig := Migration{
			Version:    version,
			Action:     action,
			Filename:   file,
			Name:       name,
			Md5:        md5sum,
			filesystem: filesystem,
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
