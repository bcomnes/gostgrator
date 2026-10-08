package gostgrator

import (
	"database/sql"
	"fmt"
	"strings"
)

// Sqlite3Client implements the Client interface for SQLite.
type Sqlite3Client struct {
	baseClient
}

// NewSqlite3Client creates a SQLite client using the supplied database.
// SchemaTable defaults to DefaultConfig.SchemaTable. The constructor selects the
// SQLite dialect regardless of Config.Driver; the caller owns db.
// SchemaTable is quoted as one literal table name, including any dots;
// embedded double quotes are escaped. It is not an attached-database qualifier.
func NewSqlite3Client(cfg Config, db *sql.DB) Client {
	cfg.Driver = "sqlite"
	if cfg.SchemaTable == "" {
		cfg.SchemaTable = DefaultConfig.SchemaTable
	}
	sqliteClient := &Sqlite3Client{
		baseClient: baseClient{
			cfg: cfg,
			db:  db,
		},
	}
	// Set function pointers.
	sqliteClient.getColumnsSqlFn = sqliteClient.getColumnsSql
	sqliteClient.getAddNameSqlFn = sqliteClient.getAddNameSql
	sqliteClient.getAddMd5SqlFn = sqliteClient.getAddMd5Sql
	sqliteClient.getAddRunAtSqlFn = sqliteClient.getAddRunAtSql
	return sqliteClient
}

// SQLite treats SchemaTable as one literal identifier, including any dots.
func quoteSQLiteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (c *Sqlite3Client) getColumnsSql() Statement {
	return Statement{
		SQL:  "SELECT name AS column_name FROM pragma_table_info(?);",
		Args: []any{c.cfg.SchemaTable},
	}
}

func (c *Sqlite3Client) getAddNameSql() string {
	return fmt.Sprintf(`
      ALTER TABLE %s
      ADD COLUMN name TEXT;
    `, c.quotedSchemaTable())
}

func (c *Sqlite3Client) getAddMd5Sql() string {
	return fmt.Sprintf(`
      ALTER TABLE %s
      ADD COLUMN md5 TEXT;
    `, c.quotedSchemaTable())
}

func (c *Sqlite3Client) getAddRunAtSql() string {
	return fmt.Sprintf(`
      ALTER TABLE %s
      ADD COLUMN run_at TIMESTAMP WITH TIME ZONE;
    `, c.quotedSchemaTable())
}
