// Package gostgrator provides database migration capabilities.
package gostgrator

import (
	"database/sql"
	"fmt"
	"strings"
)

// PostgresClient implements the Client interface for PostgreSQL.
type PostgresClient struct {
	baseClient
}

// NewPostgresClient creates a new PostgresClient.
func NewPostgresClient(cfg Config, db *sql.DB) Client {
	pgClient := &PostgresClient{
		baseClient: baseClient{
			cfg: cfg,
			db:  db,
		},
	}
	// Set function pointers for driver-specific SQL generators.
	pgClient.getColumnsSqlFn = pgClient.getColumnsSql
	pgClient.getAddNameSqlFn = pgClient.getAddNameSql
	pgClient.getAddMd5SqlFn = pgClient.getAddMd5Sql
	pgClient.getAddRunAtSqlFn = pgClient.getAddRunAtSql
	return pgClient
}

func (c *PostgresClient) getColumnsSql() string {
	var tableCatalogSql string
	if c.cfg.Database != "" {
		tableCatalogSql = fmt.Sprintf("AND table_catalog = '%s'", c.cfg.Database)
	}
	parts := strings.Split(c.cfg.SchemaTable, ".")
	tableName := parts[0]
	var schemaSql string
	if len(parts) > 1 {
		tableName = parts[1]
		schemaSql = fmt.Sprintf("AND table_schema = '%s'", parts[0])
	} else if c.cfg.CurrentSchema != "" {
		schemaSql = fmt.Sprintf("AND table_schema = '%s'", c.cfg.CurrentSchema)
	}
	return fmt.Sprintf(`
      SELECT column_name
      FROM INFORMATION_SCHEMA.COLUMNS
      WHERE table_name = '%s'
      %s
      %s;
    `, tableName, tableCatalogSql, schemaSql)
}

func (c *PostgresClient) getAddNameSql() string {
	return fmt.Sprintf(`
      ALTER TABLE %s
      ADD COLUMN name TEXT;
    `, c.quotedSchemaTable())
}

func (c *PostgresClient) getAddMd5Sql() string {
	return fmt.Sprintf(`
      ALTER TABLE %s
      ADD COLUMN md5 TEXT;
    `, c.quotedSchemaTable())
}

func (c *PostgresClient) getAddRunAtSql() string {
	return fmt.Sprintf(`
      ALTER TABLE %s
      ADD COLUMN run_at TIMESTAMP WITH TIME ZONE;
    `, c.quotedSchemaTable())
}
