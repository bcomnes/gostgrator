// Package gostgrator provides database migration capabilities.
package gostgrator

import (
	"database/sql"
	"fmt"
)

// Sqlite3Client implements the Client interface for SQLite.
type Sqlite3Client struct {
	baseClient
}

// NewSqlite3Client creates a new Sqlite3Client.
func NewSqlite3Client(cfg Config, db *sql.DB) Client {
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

func (c *Sqlite3Client) getColumnsSql() string {
	return fmt.Sprintf(`
      SELECT name AS column_name
      FROM pragma_table_info('%s');
    `, c.cfg.SchemaTable)
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
