package gostgrator

import (
	"database/sql"
	"fmt"
)

// Sqlite3Client implements Client for SQLite and embeds BaseClient.
type Sqlite3Client struct {
	BaseClient
}

// NewSqlite3Client creates a new Sqlite3Client.
func NewSqlite3Client(cfg Config, db *sql.DB) *Sqlite3Client {
	return &Sqlite3Client{
		BaseClient: BaseClient{
			Config: cfg,
			DB:     db,
		},
	}
}

// QuotedSchemaTable for SQLite is simply the table name.
func (c *Sqlite3Client) QuotedSchemaTable() string {
	return c.Config.SchemaTable
}

// getColumnsSql returns SQL to get column information using SQLite's PRAGMA.
func (c *Sqlite3Client) getColumnsSql() string {
	return fmt.Sprintf("PRAGMA table_info(%s);", c.Config.SchemaTable)
}

// getAddNameSql returns SQL to add the "name" column.
func (c *Sqlite3Client) getAddNameSql() string {
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN name TEXT;`, c.Config.SchemaTable)
}

// getAddMd5Sql returns SQL to add the "md5" column.
func (c *Sqlite3Client) getAddMd5Sql() string {
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN md5 TEXT;`, c.Config.SchemaTable)
}

// getAddRunAtSql returns SQL to add the "run_at" column.
// SQLite does not have a dedicated TIMESTAMP type so TEXT is used.
func (c *Sqlite3Client) getAddRunAtSql() string {
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN run_at TEXT;`, c.Config.SchemaTable)
}
