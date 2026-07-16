---
PLAN: "test: postgres prueba orm/conformance (DML) y ddl/conformance (DDL); su Compiler implementa ddl.Compiler"
TAG: v0.4.0
---

# PLAN — `tinywasm/postgres`: probar `orm/conformance` + `ddl/conformance`

Orquestado por [`DDL_DML_SPLIT_MASTER_PLAN.md`](https://github.com/tinywasm/app-releases/blob/main/docs/DDL_DML_SPLIT_MASTER_PLAN.md)
— **pieza #4, Ola A**. Autocontenido, en español. **Solo tienes este repo** (`github.com/tinywasm/postgres`).

> **Prerequisito:** `go install github.com/tinywasm/devflow/cmd/gotest@latest`.
> Tests con `gotest`. Publica con `gopush 'mensaje'`.

## 1. Qué se hace y por qué

Con el split DDL/DML (ver master) hay **dos** contratos ejecutables, y `postgres` (backend SQL completo)
entra en **ambos**:

- **`orm/conformance`** (DML): el SQL de datos ejecuta y round-trip correcto.
- **`ddl/conformance`** (DDL, repo `tinywasm/ddl`): el DDL crea el esquema correcto.

Y el split expone la mitad DDL del compilador como `ddl.Compiler`: hoy `translate.go`/`compiler.go`
compilan DML+DDL juntos; ahora las ramas DDL se exponen vía `CompileDDL(ddl.Stmt, model.Model)` para que
el runtime `tinywasm/ddl` las ejecute. La generación SQL **no se reescribe**, solo se mueve.

## 2. Estado verificado

- `postgres.New(dsn string) (*orm.DB, error)` (`adapter.go:33`) → `orm.New(adapter, adapter)`; falla en
  `sql.Open`/`Ping` si el DSN es malo/inalcanzable. `orm.Register("postgres", New)` (`adapter.go:12`).
- `postgres.NewCompiler() *Compiler` implementa `orm.Compiler` y `ddlc.Exporter` (`compiler.go:31`
  `ExportDDL`). `translate.go` implementa `Logic()`-aware WHERE y las ramas DDL.
- `AdapterForTest(*sql.DB) *PostgresAdapter` existe para tests. Los tests de integración leen
  `os.Getenv("POSTGRES_DSN")` y hacen `t.Skipf(...)` si no conecta (`tests/adapter_test.go:63-76`) —
  **replica ese patrón**. Paquete de tests: `package tests` bajo `tests/`.
- `postgres` ya depende de `ddlc v0.0.2`.

## 3. Cambios

### 3.1 `go.mod`
```
go get github.com/tinywasm/orm@v0.10.0
go get github.com/tinywasm/ddl@v0.0.1
go mod tidy
```

### 3.2 Implementar `ddl.Compiler`

El `*Compiler` gana `CompileDDL(s ddl.Stmt, m model.Model) (string, []any, error)` con el **mismo** SQL
Postgres que hoy produce `translate.go` para DDL (`CREATE TABLE`, `DROP TABLE`, `ALTER TABLE ADD/RENAME/
DROP COLUMN`), mapeando `ddl.Op`→SQL. Añade `var _ ddl.Compiler = postgres.NewCompiler()`. El
`orm.Compiler.Compile` conserva solo DML. No dupliques generación SQL.

### 3.3 `tests/conformance_test.go` (`package tests`) — ambas suites

```go
package tests

import (
	"database/sql"
	"os"
	"testing"

	"github.com/tinywasm/ddl"
	ddlconf "github.com/tinywasm/ddl/conformance"
	"github.com/tinywasm/model"
	"github.com/tinywasm/orm"
	ormconf "github.com/tinywasm/orm/conformance"
	"github.com/tinywasm/postgres"
)

func dsnOrSkip(t *testing.T) string {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set (see docs/POSTGRES_SETUP.md)")
	}
	probe, err := postgres.New(dsn)
	if err != nil {
		t.Skipf("POSTGRES_DSN set but unreachable: %v", err)
	}
	_ = probe.Close()
	return dsn
}

// DML: schema via ddlc.ExportDDL (DROP+CREATE for isolation), then orm data ops.
func TestPostgres_ORMConformance(t *testing.T) {
	dsn := dsnOrSkip(t)
	ormconf.Run(t, ormconf.Factory{
		Name: "postgres",
		New: func(t *testing.T, models ...model.Model) *orm.DB {
			raw, err := sql.Open("postgres", dsn)
			if err != nil { t.Fatalf("sql.Open: %v", err) }
			c := postgres.NewCompiler()
			for _, m := range models { _, _ = raw.Exec("DROP TABLE IF EXISTS " + m.ModelName()) }
			ddlSQL, err := c.ExportDDL(models)
			if err != nil { t.Fatalf("ExportDDL: %v", err) }
			if _, err := raw.Exec(ddlSQL); err != nil { t.Fatalf("apply DDL: %v", err) }
			return orm.New(postgres.AdapterForTest(raw), c)
		},
	})
}

// DDL: drive tinywasm/ddl runtime with postgres's ddl.Compiler.
func TestPostgres_DDLConformance(t *testing.T) {
	dsn := dsnOrSkip(t)
	ddlconf.Run(t, ddlconf.Factory{
		Name: "postgres",
		New: func(t *testing.T) (*ddl.DB, orm.Executor, func(string) []string) {
			raw, err := sql.Open("postgres", dsn)
			if err != nil { t.Fatalf("sql.Open: %v", err) }
			_, _ = raw.Exec("DROP TABLE IF EXISTS conformance_widget")
			exec := postgres.AdapterForTest(raw)
			schema := ddl.New(exec, postgres.NewCompiler())
			cols := func(table string) []string { /* information_schema.columns → names */ return nil }
			return schema, exec, cols
		},
	})
}
```

> Ajusta `cols` a `SELECT column_name FROM information_schema.columns WHERE table_name=$1 ORDER BY
> ordinal_position`. `AdapterForTest` te da el `orm.Executor` sobre un `*sql.DB` ya abierto. Firma exacta
> de `ddlconf.Factory` desde `tinywasm/ddl`.

## 4. Si alguna suite se pone en rojo → corregir `translate.go`/`adapter.go`

Nunca la suite. Puntos: placeholders `$1,$2` bien numerados, `sql.ErrNoRows`→`orm.ErrNoRows` en el
scanner, DDL de tipos Postgres válidos, `IN ($1,$2)`, booleanos, `ALTER TABLE ADD COLUMN`.

## 5. Criterios de aceptación

- `*Compiler` implementa `orm.Compiler` **y** `ddl.Compiler`.
- Con `POSTGRES_DSN` alcanzable: `TestPostgres_ORMConformance` y `TestPostgres_DDLConformance` verdes.
- Sin `POSTGRES_DSN`: ambos **saltan** limpio.
- `go.mod` en `orm@v0.10.0`+, `ddl@v0.0.1`+; `go mod tidy` limpio; publicado con `gopush`.

## 6. Etapas

| # | Etapa | Archivo(s) | Criterio |
|---|---|---|---|
| 1 | Bump orm+ddl | `go.mod` | `orm@v0.10.0`, `ddl@v0.0.1` |
| 2 | `CompileDDL` | `compiler.go`/`translate.go` | `var _ ddl.Compiler` |
| 3 | Test DML | `tests/conformance_test.go` | `ormconf.Run` + skip |
| 4 | Test DDL | `tests/conformance_test.go` | `ddlconf.Run` + skip |
| 5 | Correcciones (si aplica) | `translate.go`/`adapter.go` | suites verdes contra Postgres real |
| 6 | Publicar | — | `gotest` verde; `gopush 'test: orm+ddl conformance'` |

## 7. Cierre

Tras `gopush`, **borra** `docs/PLAN.md`; correcciones de comportamiento → `README.md`.
