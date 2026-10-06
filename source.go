package gostgrator

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

// MigrationSource selects a disk or filesystem migration source.
// Use DiskMigrations or FSMigrations, as a value or a non-nil pointer.
type MigrationSource interface {
	isMigrationSource()
}

// DiskMigrations selects migration files using OS filesystem paths.
type DiskMigrations struct {
	// Pattern is a filepath.Glob pattern, which may be absolute or relative.
	Pattern string
}

func (DiskMigrations) isMigrationSource() {}

// FSMigrations selects migration files from a filesystem, including embed.FS.
// The filesystem is only read; migration creation is not supported.
type FSMigrations struct {
	// FS contains the migration files and must not be nil.
	FS fs.FS
	// Pattern is an fs.Glob pattern relative to FS, using forward slashes.
	// It must not contain empty, dot, or dot-dot path components.
	Pattern string
}

func (FSMigrations) isMigrationSource() {}

// migrationSource resolves legacy configuration without changing OS path semantics.
func migrationSource(cfg Config) (fs.FS, string, error) {
	if cfg.Migrations == nil {
		return nil, cfg.MigrationPattern, nil
	}
	if cfg.MigrationPattern != "" {
		return nil, "", fmt.Errorf("Migrations and MigrationPattern must not both be set")
	}
	var filesystem fs.FS
	var pattern string
	switch source := cfg.Migrations.(type) {
	case DiskMigrations:
		pattern = source.Pattern
	case *DiskMigrations:
		if source == nil {
			return nil, "", fmt.Errorf("DiskMigrations must not be nil")
		}
		pattern = source.Pattern
	case FSMigrations:
		filesystem, pattern = source.FS, source.Pattern
		if nilFilesystem(filesystem) {
			return nil, "", fmt.Errorf("FSMigrations.FS must not be nil")
		}
	case *FSMigrations:
		if source == nil || nilFilesystem(source.FS) {
			return nil, "", fmt.Errorf("FSMigrations and its FS must not be nil")
		}
		filesystem, pattern = source.FS, source.Pattern
	default:
		return nil, "", fmt.Errorf("unsupported migration source %T", cfg.Migrations)
	}
	if pattern == "" {
		return nil, "", fmt.Errorf("migration source Pattern must not be empty")
	}
	var err error
	if filesystem != nil {
		if !fs.ValidPath(pattern) || strings.Contains(pattern, "\\") {
			return nil, "", fmt.Errorf("filesystem migration pattern must be a slash-separated root-relative path: %q", pattern)
		}
		_, err = path.Match(pattern, "")
	} else {
		_, err = filepath.Match(pattern, "")
	}
	if err != nil {
		return nil, "", fmt.Errorf("invalid migration pattern %q: %w", pattern, err)
	}
	return filesystem, pattern, nil
}

func nilFilesystem(filesystem fs.FS) bool {
	if filesystem == nil {
		return true
	}
	value := reflect.ValueOf(filesystem)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
