// Package gostgrator provides database migration capabilities.
package gostgrator

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// NewClient creates a new Client based on the provided configuration and database connection.
func NewClient(cfg Config, db *sql.DB) (Client, error) {
	switch strings.ToLower(cfg.Driver) {
	case "pg":
		return NewPostgresClient(cfg, db), nil
	case "sqlite3":
		return NewSqlite3Client(cfg, db), nil
	default:
		return nil, fmt.Errorf("db driver '%s' not supported. Must be one of: sqlite3 or pg", cfg.Driver)
	}
}

// Client defines the interface for migration clients.
type Client interface {
	RunQuery(ctx context.Context, query string) (*sql.Rows, error)
	RunSqlScript(ctx context.Context, script string) error
	GetDatabaseVersionSql() string
	HasVersionTable(ctx context.Context) (bool, error)
	EnsureTable(ctx context.Context) error
	GetMd5Sql(m Migration) string
	PersistActionSql(m Migration) string
}

// baseClient provides common functionality.
type baseClient struct {
	cfg Config
	db  *sql.DB

	// Function pointers for driver-specific SQL generators.
	getColumnsSqlFn func() string
	getAddNameSqlFn func() string
	getAddMd5SqlFn  func() string
	getAddRunAtSqlFn func() string
}

// quotedSchemaTable quotes the schemaTable if using PostgreSQL.
func (c *baseClient) quotedSchemaTable() string {
	if strings.ToLower(c.cfg.Driver) == "pg" {
		parts := strings.Split(c.cfg.SchemaTable, ".")
		for i, part := range parts {
			parts[i] = fmt.Sprintf(`"%s"`, part)
		}
		return strings.Join(parts, ".")
	}
	return c.cfg.SchemaTable
}

// RunQuery executes a query and sets search_path if needed.
func (c *baseClient) RunQuery(ctx context.Context, query string) (*sql.Rows, error) {
	if strings.ToLower(c.cfg.Driver) == "pg" && c.cfg.CurrentSchema != "" {
		_, err := c.db.ExecContext(ctx, fmt.Sprintf("SET search_path = %s", c.cfg.CurrentSchema))
		if err != nil {
			return nil, err
		}
	}
	return c.db.QueryContext(ctx, query)
}

// RunSqlScript executes a SQL script.
func (c *baseClient) RunSqlScript(ctx context.Context, script string) error {
	_, err := c.db.ExecContext(ctx, script)
	return err
}

// PersistActionSql generates SQL to record a migration action.
func (c *baseClient) PersistActionSql(m Migration) string {
	action := strings.ToLower(m.Action)
	if action == "do" {
		runAt := time.Now().UTC().Format("2006-01-02 15:04:05")
		return fmt.Sprintf(`
          INSERT INTO %s (version, name, md5, run_at)
          VALUES (%d, '%s', '%s', '%s');
        `, c.quotedSchemaTable(), m.Version, m.Name, m.Md5, runAt)
	} else if action == "undo" {
		return fmt.Sprintf(`
          DELETE FROM %s
          WHERE version = %d;
        `, c.quotedSchemaTable(), m.Version)
	}
	return fmt.Sprintf("/* unknown migration action: %s */", m.Action)
}

// GetMd5Sql returns SQL to fetch the MD5 checksum for a migration version.
func (c *baseClient) GetMd5Sql(m Migration) string {
	return fmt.Sprintf(`
      SELECT md5
      FROM %s
      WHERE version = %d;
    `, c.quotedSchemaTable(), m.Version)
}

// GetDatabaseVersionSql returns SQL to fetch the highest applied migration version.
func (c *baseClient) GetDatabaseVersionSql() string {
	return fmt.Sprintf(`
      SELECT version
      FROM %s
      ORDER BY version DESC
      LIMIT 1;
    `, c.quotedSchemaTable())
}

// HasVersionTable checks for the existence of the migration table.
func (c *baseClient) HasVersionTable(ctx context.Context) (bool, error) {
	query := c.getColumnsSqlFn()
	rows, err := c.RunQuery(ctx, query)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if rows.Next() {
		return true, nil
	}
	return false, nil
}

// EnsureTable creates the migration table if it does not exist and adds missing columns.
func (c *baseClient) EnsureTable(ctx context.Context) error {
	query := c.getColumnsSqlFn()
	rows, err := c.RunQuery(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return err
		}
		columns[strings.ToLower(colName)] = true
	}
	var sqls []string
	if len(columns) == 0 {
		colType := "BIGINT"
		if strings.ToLower(c.cfg.Driver) == "sqlite3" {
			colType = "INTEGER"
		} else if strings.ToLower(c.cfg.Driver) == "pg" {
			parts := strings.Split(c.cfg.SchemaTable, ".")
			if len(parts) > 1 {
				sqls = append(sqls, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s";`, parts[0]))
			}
		}
		sqls = append(sqls, fmt.Sprintf(`
          CREATE TABLE %s (
            version %s PRIMARY KEY
          );
        `, c.quotedSchemaTable(), colType))
		sqls = append(sqls, fmt.Sprintf(`
          INSERT INTO %s (version)
          VALUES (0);
        `, c.quotedSchemaTable()))
	}
	if !columns["name"] {
		sqls = append(sqls, c.getAddNameSqlFn())
	}
	if !columns["md5"] {
		sqls = append(sqls, c.getAddMd5SqlFn())
	}
	if !columns["run_at"] {
		sqls = append(sqls, c.getAddRunAtSqlFn())
	}
	for _, sqlStmt := range sqls {
		if _, err := c.db.ExecContext(ctx, sqlStmt); err != nil {
			return err
		}
	}
	return nil
}
