package gostgrator

import (
	"database/sql"
	"strings"
	"testing"
)

func TestBuiltInClientConstructorDefaultsAndDialect(t *testing.T) {
	for _, tc := range []struct {
		name      string
		newClient func(Config, *sql.DB) Client
		dialect   string
	}{
		{"postgres", NewPostgresClient, "pg"},
		{"sqlite", NewSqlite3Client, "sqlite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, driver := range []string{"", "unsupported", "pg", "sqlite", "sqlite3"} {
				t.Run("driver="+driver, func(t *testing.T) {
					for _, table := range []string{"", "custom_versions", "custom_schema.versions"} {
						t.Run("table="+table, func(t *testing.T) {
							cfg := Config{Driver: driver, SchemaTable: table}
							client := tc.newClient(cfg, nil)
							var base *baseClient
							switch c := client.(type) {
							case *PostgresClient:
								base = &c.baseClient
							case *Sqlite3Client:
								base = &c.baseClient
							default:
								t.Fatalf("unexpected client type %T", client)
							}
							wantTable := table
							if wantTable == "" {
								wantTable = DefaultConfig.SchemaTable
							}
							if base.cfg.Driver != tc.dialect || base.cfg.SchemaTable != wantTable {
								t.Fatalf("client config = %+v; want dialect %q and table %q", base.cfg, tc.dialect, wantTable)
							}
							if cfg.Driver != driver || cfg.SchemaTable != table {
								t.Fatalf("constructor mutated caller config: %+v", cfg)
							}
							quoted := wantTable
							if tc.dialect == "pg" {
								quoted = `"` + strings.ReplaceAll(wantTable, ".", `"."`) + `"`
							}
							for name, query := range map[string]string{
								"version":      client.GetDatabaseVersionSql(),
								"checksum":     client.GetMd5Sql(Migration{Version: 1}),
								"persist":      client.PersistActionSql(Migration{Version: 1, Action: "do"}),
								"undo":         client.PersistActionSql(Migration{Version: 1, Action: "undo"}),
								"add name":     base.getAddNameSqlFn(),
								"add checksum": base.getAddMd5SqlFn(),
								"add run time": base.getAddRunAtSqlFn(),
							} {
								if !strings.Contains(query, " "+quoted+" ") && !strings.Contains(query, " "+quoted+"\n") {
									t.Errorf("%s SQL = %q; want table %s", name, query, quoted)
								}
							}
							columns := base.getColumnsSqlFn()
							if tc.dialect == "sqlite" {
								if !strings.Contains(columns, "pragma_table_info('"+wantTable+"')") {
									t.Errorf("column SQL = %q", columns)
								}
							} else {
								parts := strings.Split(wantTable, ".")
								if !strings.Contains(columns, "INFORMATION_SCHEMA.COLUMNS") || !strings.Contains(columns, "table_name = '"+parts[len(parts)-1]+"'") {
									t.Errorf("column SQL = %q", columns)
								}
								if len(parts) > 1 && !strings.Contains(columns, "table_schema = '"+parts[0]+"'") {
									t.Errorf("column SQL missing schema: %q", columns)
								}
							}
						})
					}
				})
			}
		})
	}
}
