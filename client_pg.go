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

// NewPostgresClient creates a PostgreSQL client using the supplied database.
// SchemaTable defaults to DefaultConfig.SchemaTable. The constructor selects the
// PostgreSQL dialect regardless of Config.Driver; the caller owns db.
// SchemaTable is an unquoted table or schema.table name. Each dot-separated
// component is quoted independently; embedded double quotes are escaped.
func NewPostgresClient(cfg Config, db *sql.DB) Client {
	cfg.Driver = "pg"
	if cfg.SchemaTable == "" {
		cfg.SchemaTable = DefaultConfig.SchemaTable
	}
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

func quotePostgresIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (c *PostgresClient) getColumnsSql() Statement {
	parts := strings.Split(c.cfg.SchemaTable, ".")
	tableName := parts[len(parts)-1]
	statement := Statement{
		SQL:  "SELECT column_name FROM INFORMATION_SCHEMA.COLUMNS WHERE table_name = $1",
		Args: []any{tableName},
	}
	if len(parts) > 1 {
		statement.SQL += " AND table_schema = $2"
		statement.Args = append(statement.Args, parts[0])
	} else {
		// Resolve the same visible relation as an unqualified, quoted table reference.
		statement.SQL += ` AND table_schema = (
					SELECT n.nspname
					FROM pg_catalog.pg_class c
					JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
					WHERE c.oid = pg_catalog.to_regclass($2)
				)`
		statement.Args = append(statement.Args, c.quotedSchemaTable())
	}
	return statement
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
