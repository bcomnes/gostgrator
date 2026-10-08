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
	case "sqlite", "sqlite3":
		return NewSqlite3Client(cfg, db), nil
	default:
		return nil, fmt.Errorf("db driver '%s' not supported. Must be one of: sqlite, sqlite3, or pg", cfg.Driver)
	}
}

func isSQLiteDriver(driver string) bool {
	switch strings.ToLower(driver) {
	case "sqlite", "sqlite3":
		return true
	default:
		return false
	}
}

// Statement contains SQL text and the arguments to bind when executing it.
// It is not a prepared statement (sql.Stmt) and holds no database resources.
// Placeholders are dialect-specific; identifiers must be quoted by the client,
// not passed in Args.
type Statement struct {
	// SQL is the query text, with placeholders matching the client's dialect.
	SQL string
	// Args contains values forwarded to QueryContext or ExecContext.
	// It may be nil when SQL requires no bound arguments.
	Args []any
}

// Client defines SQL execution and database-specific migration bookkeeping.
// Implementations can be supplied to NewGostgrator to support custom
// dialects or wrap an existing client. The client is responsible for its tracking
// table configuration; Gostgrator does not configure or close an injected client.
// SQL builders return statements with bound bookkeeping values. Execution methods
// must forward args to the driver; migration scripts are executed without args.
type Client interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, script string, args ...any) (sql.Result, error)
	GetDatabaseVersionSql() Statement
	HasVersionTable(ctx context.Context) (bool, error)
	EnsureTable(ctx context.Context) error
	GetMd5Sql(m Migration) Statement
	PersistActionSql(m Migration) Statement
}

// baseClient provides common functionality.
type baseClient struct {
	cfg Config
	db  *sql.DB

	// Function pointers for driver-specific SQL generators.
	getColumnsSqlFn  func() Statement
	getAddNameSqlFn  func() string
	getAddMd5SqlFn   func() string
	getAddRunAtSqlFn func() string
}

// quotedSchemaTable applies the selected dialect's identifier rules.
func (c *baseClient) quotedSchemaTable() string {
	if c.cfg.Driver == "pg" {
		parts := strings.Split(c.cfg.SchemaTable, ".")
		for i, part := range parts {
			parts[i] = quotePostgresIdentifier(part)
		}
		return strings.Join(parts, ".")
	}
	return quoteSQLiteIdentifier(c.cfg.SchemaTable)
}

func (c *baseClient) placeholder(position int) string {
	if c.cfg.Driver == "pg" {
		return fmt.Sprintf("$%d", position)
	}
	return "?"
}

// Exposes the QueryContext method from the configured db connection.
func (c *baseClient) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.db.QueryContext(ctx, query, args...)
}

// Exposing ExecContext from the configured db connection.
func (c *baseClient) ExecContext(ctx context.Context, script string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, script, args...)
}

// PersistActionSql generates a statement to record a migration action.
func (c *baseClient) PersistActionSql(m Migration) Statement {
	switch strings.ToLower(m.Action) {
	case "do":
		return Statement{
			SQL: fmt.Sprintf("INSERT INTO %s (version, name, md5, run_at) VALUES (%s, %s, %s, %s);",
				c.quotedSchemaTable(), c.placeholder(1), c.placeholder(2), c.placeholder(3), c.placeholder(4)),
			Args: []any{m.Version, m.Name, m.Md5, time.Now().UTC().Format("2006-01-02 15:04:05")},
		}
	case "undo":
		return Statement{
			SQL:  fmt.Sprintf("DELETE FROM %s WHERE version = %s;", c.quotedSchemaTable(), c.placeholder(1)),
			Args: []any{m.Version},
		}
	default:
		return Statement{SQL: "/* unknown migration action */"}
	}
}

// GetMd5Sql returns a statement to fetch the checksum for a bound migration version.
func (c *baseClient) GetMd5Sql(m Migration) Statement {
	return Statement{
		SQL:  fmt.Sprintf("SELECT md5 FROM %s WHERE version = %s;", c.quotedSchemaTable(), c.placeholder(1)),
		Args: []any{m.Version},
	}
}

// GetDatabaseVersionSql returns a statement to fetch the highest applied version.
func (c *baseClient) GetDatabaseVersionSql() Statement {
	return Statement{SQL: fmt.Sprintf("SELECT version FROM %s ORDER BY version DESC LIMIT 1;", c.quotedSchemaTable())}
}

// HasVersionTable checks for the existence of the migration table.
func (c *baseClient) HasVersionTable(ctx context.Context) (bool, error) {
	query := c.getColumnsSqlFn()
	rows, err := c.QueryContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if rows.Next() {
		return true, nil
	}
	return false, rows.Err()
}

// EnsureTable creates the migration table if it does not exist and adds missing columns.
func (c *baseClient) EnsureTable(ctx context.Context) error {
	query := c.getColumnsSqlFn()
	rows, err := c.QueryContext(ctx, query.SQL, query.Args...)
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
	if err := rows.Err(); err != nil {
		return err
	}
	var sqls []string
	if len(columns) == 0 {
		colType := "BIGINT"
		if isSQLiteDriver(c.cfg.Driver) {
			colType = "INTEGER"
		} else if strings.ToLower(c.cfg.Driver) == "pg" {
			parts := strings.Split(c.cfg.SchemaTable, ".")
			if len(parts) > 1 {
				sqls = append(sqls, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s;`, quotePostgresIdentifier(parts[0])))
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
		if _, err := c.ExecContext(ctx, sqlStmt); err != nil {
			return err
		}
	}
	return nil
}
