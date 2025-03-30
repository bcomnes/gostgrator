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

// BaseClient provides the common implementation.
type BaseClient struct {
	Config Config
	DB     *sql.DB
}


// RunQuery executes a query. For Postgres, if CurrentSchema is set,
// it first sets the search_path.
func (c *BaseClient) RunQuery(ctx context.Context, query string) (*sql.Rows, error) {
	if strings.ToLower(c.Config.Driver) == "pg" && c.Config.CurrentSchema != "" {
		_, err := c.DB.ExecContext(ctx, fmt.Sprintf("SET search_path = %s", c.Config.CurrentSchema))
		if err != nil {
			return nil, err
		}
	}
	return c.DB.QueryContext(ctx, query)
}

// RunSqlScript executes a SQL script.
func (c *BaseClient) RunSqlScript(ctx context.Context, script string) error {
	_, err := c.DB.ExecContext(ctx, script)
	return err
}

// QuotedSchemaTable returns the schema table name.
// The default implementation returns the table name as provided.
// Subclasses should override this if needed.
func (c *BaseClient) QuotedSchemaTable() string {
	return c.Config.SchemaTable
}

// PersistActionSql returns the SQL for persisting a migration action.
func (c *BaseClient) PersistActionSql(m Migration) string {
	qt := c.QuotedSchemaTable()
	now := time.Now().Format("2006-01-02 15:04:05")
	if strings.ToLower(m.Action) == "do" {
		return fmt.Sprintf(`
          INSERT INTO %s (version, name, md5, run_at)
          VALUES (%d, '%s', '%s', '%s');`, qt, m.Version, m.Name, m.Md5, now)
	} else if strings.ToLower(m.Action) == "undo" {
		return fmt.Sprintf(`
          DELETE FROM %s
          WHERE version = %d;`, qt, m.Version)
	}
	return ""
}

// GetMd5Sql returns SQL to fetch the md5 checksum for a migration.
func (c *BaseClient) GetMd5Sql(m Migration) string {
	qt := c.QuotedSchemaTable()
	return fmt.Sprintf(`
      SELECT md5
      FROM %s
      WHERE version = %d;`, qt, m.Version)
}

// GetDatabaseVersionSql returns SQL to get the latest version.
func (c *BaseClient) GetDatabaseVersionSql() string {
	qt := c.QuotedSchemaTable()
	return fmt.Sprintf(`
      SELECT version
      FROM %s
      ORDER BY version DESC
      LIMIT 1;`, qt)
}

// --- Dummy implementations to be overridden by the concrete client ---

// getColumnsSql returns SQL to fetch column metadata for the version table.
func (c *BaseClient) getColumnsSql() string {
	return ""
}

// getAddNameSql returns SQL to add the "name" column.
func (c *BaseClient) getAddNameSql() string {
	return ""
}

// getAddMd5Sql returns SQL to add the "md5" column.
func (c *BaseClient) getAddMd5Sql() string {
	return ""
}

// getAddRunAtSql returns SQL to add the "run_at" column.
func (c *BaseClient) getAddRunAtSql() string {
	return ""
}


// HasVersionTable checks for the existence of the version table by querying its columns.
func (c *BaseClient) HasVersionTable(ctx context.Context) (bool, error) {
	sqlStr := c.getColumnsSql()
	rows, err := c.DB.QueryContext(ctx, sqlStr)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	// if there is at least one row then we assume the table exists.
	if rows.Next() {
		return true, nil
	}
	return false, nil
}

// Helper function to check for a column name (case insensitive).
func hasColumn(columns []string, name string) bool {
	for _, col := range columns {
		if strings.EqualFold(col, name) {
			return true
		}
	}
	return false
}

// EnsureTable checks if the version table exists and creates/updates it if necessary.
func (c *BaseClient) EnsureTable(ctx context.Context) error {
	var columns []string
	sqlStr := c.getColumnsSql()
	rows, err := c.DB.QueryContext(ctx, sqlStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	// For Postgres, the query returns one column (column_name).
	// For SQLite, PRAGMA table_info returns multiple rows; assume the first column holds the name.
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return err
		}
		columns = append(columns, col)
	}

	var queries []string
	// If no columns are returned, assume the table does not exist.
	if len(columns) == 0 {
		var colType string
		if strings.ToLower(c.Config.Driver) == "pg" {
			// If SchemaTable contains a dot, create the schema first.
			if strings.Contains(c.Config.SchemaTable, ".") {
				parts := strings.Split(c.Config.SchemaTable, ".")
				queries = append(queries, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s";`, parts[0]))
			}
			colType = "BIGINT"
		} else if strings.ToLower(c.Config.Driver) == "sqlite3" {
			colType = "INTEGER"
		} else {
			colType = "BIGINT"
		}
		queries = append(queries, fmt.Sprintf(`
          CREATE TABLE %s (
            version %s PRIMARY KEY
          );`, c.QuotedSchemaTable(), colType))
		queries = append(queries, fmt.Sprintf(`
          INSERT INTO %s (version)
          VALUES (0);`, c.QuotedSchemaTable()))
	}

	// Check for missing columns: name, md5, run_at.
	if !hasColumn(columns, "name") {
		queries = append(queries, c.getAddNameSql())
	}
	if !hasColumn(columns, "md5") {
		queries = append(queries, c.getAddMd5Sql())
	}
	if !hasColumn(columns, "run_at") {
		queries = append(queries, c.getAddRunAtSql())
	}

	for _, q := range queries {
		if _, err := c.DB.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
