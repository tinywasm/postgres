package tests

import (
	"database/sql"
	"os"
	"testing"

	"webtyp.com/ddl"
	ddlconf "webtyp.com/ddl/conformance"
	"webtyp.com/model"
	"webtyp.com/postgres"
	"webtyp.com/storage"
	dbconf "webtyp.com/storage/conformance"
)

func dsnOrSkip(t *testing.T) string {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set (see docs/POSTGRES_SETUP.md)")
	}
	probe, err := postgres.Open(dsn)
	if err != nil {
		t.Skipf("POSTGRES_DSN set but unreachable: %v", err)
	}
	_ = probe.Close()
	return dsn
}

// DML: schema via Compiler.ExportDDL (DROP+CREATE for isolation), then storage data ops.
func TestPostgres_DBConformance(t *testing.T) {
	dsn := dsnOrSkip(t)
	dbconf.Run(t, dbconf.Factory{
		Name: "postgres",
		New: func(t *testing.T, models ...model.Model) storage.Conn {
			raw, err := sql.Open("postgres", dsn)
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			c := postgres.NewCompiler()
			for _, m := range models {
				_, _ = raw.Exec("DROP TABLE IF EXISTS " + m.ModelName() + " CASCADE")
			}
			ddlSQL, err := c.ExportDDL(models)
			if err != nil {
				t.Fatalf("ExportDDL: %v", err)
			}
			if _, err := raw.Exec(ddlSQL); err != nil {
				t.Fatalf("apply DDL: %v", err)
			}
			return postgres.AdapterForTest(raw)
		},
	})
}

// DDL: drive webtyp/ddl runtime with postgres's ddl.Compiler (PostgresAdapter itself).
func TestPostgres_DDLConformance(t *testing.T) {
	dsn := dsnOrSkip(t)
	ddlconf.Run(t, ddlconf.Factory{
		Name: "postgres",
		New: func(t *testing.T) (schema *ddl.DB, conn storage.Conn, cols func(string) []string) {
			raw, err := sql.Open("postgres", dsn)
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			_, _ = raw.Exec("DROP TABLE IF EXISTS conformance_widget CASCADE")
			adapter := postgres.AdapterForTest(raw)
			schema = ddl.New(adapter, adapter) // PostgresAdapter is both storage.Conn and ddl.Compiler
			cols = func(table string) []string {
				rows, err := raw.Query(`SELECT column_name FROM information_schema.columns WHERE table_name = $1 ORDER BY ordinal_position`, table)
				if err != nil {
					t.Fatalf("cols query failed: %v", err)
				}
				defer rows.Close()
				var names []string
				for rows.Next() {
					var name string
					if err := rows.Scan(&name); err != nil {
						t.Fatalf("cols scan failed: %v", err)
					}
					names = append(names, name)
				}
				if err := rows.Err(); err != nil {
					t.Fatalf("cols rows error: %v", err)
				}
				return names
			}
			return schema, adapter, cols
		},
	})
}
