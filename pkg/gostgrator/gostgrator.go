// Package gostgrator provides database migration capabilities.
package gostgrator

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Config holds settings for migrations.
type Config struct {
	// Driver is the database driver, e.g., "pg" or "sqlite3".
	Driver string

	// Database is the database name (used by some clients, e.g. PostgreSQL).
	Database string

	// SchemaTable is the name of the migration table.
	SchemaTable string

	// MigrationPattern is the glob pattern for migration files (e.g. "./migrations/*.sql").
	MigrationPattern string

	// Newline is the desired newline style ("LF", "CR", or "CRLF").
	Newline string

	// CurrentSchema is used for PostgreSQL if SchemaTable doesn’t include a dot.
	CurrentSchema string

	// ValidateChecksums indicates if the tool should validate migration checksums.
	ValidateChecksums bool
}

// DefaultConfig provides default values for configuration.
var DefaultConfig = Config{
	SchemaTable:       "schemaversion",
	ValidateChecksums: true,
}

// Gostgrator is the main orchestrator for running database migrations.
//
// It loads migration files, determines the current database version,
// validates checksums (if enabled), and runs the necessary migrations to reach a target version.
type Gostgrator struct {
	cfg        Config
	migrations []Migration
	client     Client
}

// NewGostgrator creates a new Gostgrator instance with the provided configuration and database connection.
func NewGostgrator(cfg Config, db *sql.DB) (*Gostgrator, error) {
	// Merge defaults.
	if cfg.SchemaTable == "" {
		cfg.SchemaTable = DefaultConfig.SchemaTable
	}
	if !cfg.ValidateChecksums {
		cfg.ValidateChecksums = DefaultConfig.ValidateChecksums
	}
	client, err := NewClient(cfg, db)
	if err != nil {
		return nil, err
	}
	return &Gostgrator{
		cfg:    cfg,
		client: client,
	}, nil
}

// GetMigrations scans for migration files and loads them into Gostgrator.
func (g *Gostgrator) GetMigrations() error {
	migs, err := GetMigrations(g.cfg)
	if err != nil {
		return err
	}
	g.migrations = migs
	return nil
}

// RunQuery is a helper to execute a query using the underlying client.
func (g *Gostgrator) RunQuery(ctx context.Context, query string) (*sql.Rows, error) {
	return g.client.RunQuery(ctx, query)
}

// RunSqlScript executes a SQL script using the underlying client.
func (g *Gostgrator) RunSqlScript(ctx context.Context, script string) error {
	return g.client.RunSqlScript(ctx, script)
}

// GetDatabaseVersion returns the current database version.
// If the migration table is not initialized, it returns 0.
func (g *Gostgrator) GetDatabaseVersion(ctx context.Context) (int, error) {
	versionSql := g.client.GetDatabaseVersionSql()
	initialized, err := g.client.HasVersionTable(ctx)
	if err != nil {
		return 0, err
	}
	if !initialized {
		return 0, nil
	}
	rows, err := g.client.RunQuery(ctx, versionSql)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var version int
	if rows.Next() {
		if err := rows.Scan(&version); err != nil {
			return 0, err
		}
	}
	return version, nil
}

// GetMaxVersion returns the highest migration version available.
func (g *Gostgrator) GetMaxVersion() (int, error) {
	if len(g.migrations) == 0 {
		if err := g.GetMigrations(); err != nil {
			return 0, err
		}
	}
	max := 0
	for _, m := range g.migrations {
		if m.Version > max {
			max = m.Version
		}
	}
	return max, nil
}

// ValidateMigrations verifies that applied migrations have not changed by comparing MD5 checksums.
func (g *Gostgrator) ValidateMigrations(ctx context.Context, databaseVersion int) error {
	if err := g.GetMigrations(); err != nil {
		return err
	}
	for _, m := range g.migrations {
		if m.Action == "do" && m.Version > 0 && m.Version <= databaseVersion {
			query := g.client.GetMd5Sql(m)
			rows, err := g.client.RunQuery(ctx, query)
			if err != nil {
				return err
			}
			var dbMd5 sql.NullString
			if rows.Next() {
				if err := rows.Scan(&dbMd5); err != nil {
					rows.Close()
					return err
				}
			}
			rows.Close()
			if dbMd5.Valid && m.Md5 != "" && dbMd5.String != m.Md5 {
				return fmt.Errorf("MD5 checksum failed for migration [%d]", m.Version)
			}
		}
	}
	return nil
}

// RunMigrations applies the provided migrations in sequence.
func (g *Gostgrator) RunMigrations(ctx context.Context, migrations []Migration) ([]Migration, error) {
	var applied []Migration
	for _, m := range migrations {
		sqlScript, err := m.GetSQL()
		if err != nil {
			return applied, err
		}
		if err := g.client.RunSqlScript(ctx, sqlScript); err != nil {
			return applied, err
		}
		persistSQL := g.client.PersistActionSql(m)
		if _, err := g.client.RunQuery(ctx, persistSQL); err != nil {
			return applied, err
		}
		applied = append(applied, m)
	}
	return applied, nil
}

func (g *Gostgrator) GetRunnableMigrations(databaseVersion, targetVersion int) ([]Migration, error) {
	if targetVersion > databaseVersion {
		var runnable []Migration
		for _, m := range g.migrations {
			if m.Action == "do" && m.Version > databaseVersion && m.Version <= targetVersion {
				runnable = append(runnable, m)
			}
		}
		sortMigrationsAsc(runnable)
		return runnable, nil
	}

	if targetVersion < databaseVersion {
		var runnable []Migration
		for _, m := range g.migrations {
			if m.Action == "undo" && m.Version <= databaseVersion && m.Version > targetVersion {
				runnable = append(runnable, m)
			}
		}
		sortMigrationsDesc(runnable)
		return runnable, nil
	}

	// targetVersion == databaseVersion
	return nil, nil
}

// Migrate moves the schema to the target version.
// If target is "max" or empty, it migrates to the highest available version.
func (g *Gostgrator) Migrate(ctx context.Context, target string) ([]Migration, error) {
	if err := g.client.EnsureTable(ctx); err != nil {
		return nil, err
	}
	if err := g.GetMigrations(); err != nil {
		return nil, err
	}
	var targetVersion int
	var err error
	cleaned := strings.ToLower(strings.TrimSpace(target))
	if cleaned == "max" || cleaned == "" {
		targetVersion, err = g.GetMaxVersion()
		if err != nil {
			return nil, err
		}
	} else {
		targetVersion, err = strconv.Atoi(cleaned)
		if err != nil {
			return nil, fmt.Errorf("invalid target version: %v", err)
		}
	}
	dbVersion, err := g.GetDatabaseVersion(ctx)
	if err != nil {
		return nil, err
	}
	if g.cfg.ValidateChecksums && targetVersion >= dbVersion {
		if err := g.ValidateMigrations(ctx, dbVersion); err != nil {
			return nil, err
		}
	}
	runnable, err := g.GetRunnableMigrations(dbVersion, targetVersion)
	if err != nil {
		return nil, err
	}
	applied, err := g.RunMigrations(ctx, runnable)
	if err != nil {
		return applied, err
	}
	return applied, nil
}
