package gostgrator

import (
	"database/sql"
	"reflect"
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
					for _, table := range []string{"", "custom_versions", "custom_schema.versions", `schema '".table '"`} {
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
							quoted := `"` + strings.ReplaceAll(wantTable, `"`, `""`) + `"`
							if tc.dialect == "pg" {
								quoted = `"` + strings.ReplaceAll(strings.ReplaceAll(wantTable, `"`, `""`), ".", `"."`) + `"`
							}
							for name, query := range map[string]string{
								"version":      client.GetDatabaseVersionSql().SQL,
								"checksum":     client.GetMd5Sql(Migration{Version: 1}).SQL,
								"persist":      client.PersistActionSql(Migration{Version: 1, Action: "do"}).SQL,
								"undo":         client.PersistActionSql(Migration{Version: 1, Action: "undo"}).SQL,
								"add name":     base.getAddNameSqlFn(),
								"add checksum": base.getAddMd5SqlFn(),
								"add run time": base.getAddRunAtSqlFn(),
							} {
								if !strings.Contains(query, " "+quoted+" ") && !strings.Contains(query, " "+quoted+"\n") {
									t.Errorf("%s SQL = %q; want table %s", name, query, quoted)
								}
							}
							unknown := client.PersistActionSql(Migration{Action: "*/; DROP TABLE versions; --"})
							if unknown.SQL != "/* unknown migration action */" || len(unknown.Args) != 0 {
								t.Errorf("unknown action leaked into SQL: %+v", unknown)
							}
							columns := base.getColumnsSqlFn()
							if tc.dialect == "sqlite" {
								if !strings.Contains(columns.SQL, "pragma_table_info(?)") || !reflect.DeepEqual(columns.Args, []any{wantTable}) {
									t.Errorf("column statement = %+v", columns)
								}
							} else {
								parts := strings.Split(wantTable, ".")
								schema := quoted
								if len(parts) > 1 {
									schema = parts[0]
								}
								if !strings.Contains(columns.SQL, "table_name = $1") || !reflect.DeepEqual(columns.Args, []any{parts[len(parts)-1], schema}) {
									t.Errorf("column statement = %+v", columns)
								}
								if len(parts) > 1 && !strings.Contains(columns.SQL, "table_schema = $2") {
									t.Errorf("column SQL missing schema: %+v", columns)
								}
							}
						})
					}
				})
			}
		})
	}
}
