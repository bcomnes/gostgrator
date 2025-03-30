package gostgrator

import (
	"database/sql"
	"fmt"
	"strings"
)

// PostgresClient implements Client for PostgreSQL and embeds BaseClient.
type PostgresClient struct {
	BaseClient
}

// NewPostgresClient creates a new PostgresClient.
func NewPostgresClient(cfg Config, db *sql.DB) *PostgresClient {
	return &PostgresClient{
		BaseClient: BaseClient{
			Config: cfg,
			DB:     db,
		},
	}
}

// QuotedSchemaTable returns the schema table name with each part quoted.
func (c *PostgresClient) QuotedSchemaTable() string {
	parts := strings.Split(c.Config.SchemaTable, ".")
	for i, part := range parts {
		parts[i] = fmt.Sprintf(`"%s"`, part)
	}
	return strings.Join(parts, ".")
}

// getColumnsSql returns SQL to list columns for the version table in Postgres.
func (c *PostgresClient) getColumnsSql() string {
	var schema, table string
	if strings.Contains(c.Config.SchemaTable, ".") {
		parts := strings.Split(c.Config.SchemaTable, ".")
		schema = parts[0]
		table = parts[1]
	} else {
		schema = "public"
		table = c.Config.SchemaTable
	}
	return fmt.Sprintf(`SELECT column_name FROM information_schema.columns WHERE table_schema = '%s' AND table_name = '%s';`, schema, table)
}

// getAddNameSql returns SQL to add the "name" column.
func (c *PostgresClient) getAddNameSql() string {
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN name TEXT;`, c.QuotedSchemaTable())
}

// getAddMd5Sql returns SQL to add the "md5" column.
func (c *PostgresClient) getAddMd5Sql() string {
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN md5 TEXT;`, c.QuotedSchemaTable())
}

// getAddRunAtSql returns SQL to add the "run_at" column.
func (c *PostgresClient) getAddRunAtSql() string {
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN run_at TIMESTAMP;`, c.QuotedSchemaTable())
}
