---
PLAN: "refactor!: postgres implementa storage.Conn + ddl.Compiler (contrato movido de orm a tinywasm/storage)"
TAG: v0.4.0
STATUS: running
SESSION: 13716118547241455738
---

# PLAN — `tinywasm/postgres`: migrar de `orm.Compiler`/`orm.Executor` a `storage.Conn` + `ddl.Compiler`

Orquestado por
[`DB_PORT_MASTER_PLAN.md`](https://github.com/tinywasm/app-releases/blob/main/docs/DB_PORT_MASTER_PLAN.md)
— **pieza #5**. Autocontenido, en español. **Solo tienes este repo** (`github.com/tinywasm/postgres`).

> **Prerequisito:** `go install github.com/tinywasm/devflow/cmd/gotest@latest`.
> Tests con `gotest`. Publica con `gopush 'mensaje'`.
> Este plan **requiere `tinywasm/storage@v0.0.2` y `tinywasm/ddl@v0.0.2` ya publicados**. Si no resuelven
> en `go get`, para y repórtalo.

## 0. Qué cambió respecto a la versión anterior de este plan

Antes: `postgres` iba a implementar `orm.Compiler` (DML) + `ddl.Compiler` (DDL), probando
`orm/conformance`+`ddl/conformance`, y seguía registrándose vía `orm.Register("postgres", New)`. Eso
asumía que `orm` seguía siendo dueño del contrato. Ya no lo es — se extrajo a `tinywasm/storage`. Ahora:

- `PostgresAdapter` implementa **`storage.Conn`** (Executor+Compiler unidos) en vez de `orm.Executor`+
  `orm.Compiler` por separado, y **`ddl.Compiler`** para DDL.
- **Sin `init()`/`orm.Register`.** `postgres.New(dsn)` pasa a `postgres.Open(dsn) (storage.Conn, error)` —
  construcción explícita, sin registro por string (ver `tinywasm/storage`'s AGENTS.md, sección "No DSN
  registry", y `DB_PORT_PROPOSAL.md` §6.6).
- Se prueba contra **`storage/conformance`** (no `orm/conformance`) + `ddl/conformance`.
- `go.mod` final: `storage`+`ddl`+`ddlc`+`model`+`fmt`+`lib/pq`. **Cero `tinywasm/orm`.**

## 1. Qué se hace y por qué

`postgres`, a diferencia de `sqlt`/`sqlite` (compilador y adapter en repos separados), es un **backend
completo en un solo repo**: `PostgresAdapter` abre la conexión, ejecuta, y compila. Entra en **ambos**
contratos ejecutables porque es un backend SQL completo:

- **`storage/conformance`** (DML): el SQL de datos que genera ejecuta y da round-trip correcto.
- **`ddl/conformance`** (DDL): el DDL que genera crea el esquema correcto.

## 2. Estado verificado (código actual, antes de este plan)

- `adapter.go:11` `init() { orm.Register("postgres", New) }` — **se borra entero**.
- `adapter.go:33` `New(dataSourceName string) (*orm.DB, error)` → abre `*sql.DB`, construye
  `PostgresAdapter`, devuelve `orm.New(adapter, adapter)` (el mismo `*PostgresAdapter` sirve de
  executor y compilador — ya está unificado en un tipo, lo cual encaja bien con `storage.Conn`).
- `adapter.go:46` `AdapterForTest(db *sql.DB) *PostgresAdapter` — constructor de test que salta
  `sql.Open`, usado por los tests de conformidad. Se queda, cambia lo que implementa (§3.2).
- `adapter.go:51,63,69,74,79` `PostgresAdapter.Compile/Exec/QueryRow/Query/Close` — implementan
  `orm.Compiler`+`orm.Executor` hoy.
- `adapter.go:83,102` `tableColumns`/`PostgresAdapter.TableColumns` → `orm.TableIntrospector`.
- `compiler.go:11` `type Compiler struct{}` — **tipo separado**, no el mismo que `PostgresAdapter`, con
  su propio `Compile`+`ExportDDL` (`compiler.go:19,31`). Nota: hay dos tipos con un método `Compile`
  hoy (`PostgresAdapter` y `Compiler`) — no es un error de este plan, es el estado actual del repo;
  este plan migra ambos a los tipos nuevos sin resolver esa duplicación (fuera de alcance, no la
  "arregles" de paso).
- `introspect.go:9,33` `tables`/`columns` (funciones libres) + `PostgresAdapter.Tables/Columns`
  (`introspect.go:76,81`) → `orm.SchemaInspector`.
- `translate.go:47` `translate(q orm.Query, m model.Model)` — **un solo switch** que maneja DML
  (`ActionCreate/ReadOne/ReadAll/Update/Delete`) y DDL (`ActionCreateTable/DropTable/AddColumn/
  RenameColumn/DropColumn`) juntos, igual que `sqlt`. Se separa en dos (§3.3), mismo patrón que
  `sqlt/docs/PLAN.md` §3.3 — **el SQL generado dentro de cada rama no cambia**, solo el despacho.
- `translate.go:251` `Translate(q orm.Query, m model.Model)` — export público, se divide igual.
- `tx.go`: `PostgresTx` implementa `orm.Compiler`+`orm.Executor`+`Commit`+`Rollback`+
  `TableColumns`+`Tables`+`Columns` — el equivalente transaccional de `PostgresAdapter`. Migra igual.
- `go.mod`: `lib/pq@v1.11.2`, `orm@v0.9.27`, `ddlc@v0.0.2`.

## 3. Cambios

### 3.1 `go.mod`

```
go get github.com/tinywasm/storage@v0.0.1
go get github.com/tinywasm/ddl@v0.0.1
go mod tidy   # quita github.com/tinywasm/orm por completo
```

### 3.2 `adapter.go` — sin registro, `PostgresAdapter` como `storage.Conn` completo

```go
package postgres

import (
	"database/sql"

	"github.com/tinywasm/ddl"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/storage"

	_ "github.com/lib/pq"
)

type PostgresAdapter struct {
	db *sql.DB
}

// Open creates a new postgres connection and returns it as a storage.Conn. No registry, no init() —
// construct explicitly: conn, err := postgres.Open(dsn); d := orm.New(conn) (ergonomic layer)
// or use conn directly (e.g. ddl.New(conn, conn) — PostgresAdapter implements both storage.Compiler
// and ddl.Compiler, see §3.3).
func Open(dataSourceName string) (storage.Conn, error) {
	raw, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, fmt.Errf("failed to open postgres connection: %v", err)
	}
	if err := raw.Ping(); err != nil {
		return nil, fmt.Errf("failed to ping postgres: %v", err)
	}
	return &PostgresAdapter{db: raw}, nil
}

// AdapterForTest wraps an already-open *sql.DB, skipping sql.Open — used by conformance tests
// that manage the connection lifecycle themselves.
func AdapterForTest(raw *sql.DB) *PostgresAdapter {
	return &PostgresAdapter{db: raw}
}

func (p *PostgresAdapter) Compile(q storage.Query, m model.Model) (storage.Plan, error) {
	sqlStr, args, err := translate(q, m)
	if err != nil {
		return storage.Plan{}, err
	}
	return storage.Plan{Mode: q.Action, Query: sqlStr, Args: args}, nil
}

// CompileDDL implements ddl.Compiler — PostgresAdapter satisfies both compiler contracts in
// the same type, so a single Open(dsn) result works for orm.New(conn) AND ddl.New(conn, conn).
func (p *PostgresAdapter) CompileDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	return translateDDL(s, m)
}

func (p *PostgresAdapter) Exec(query string, args ...any) error {
	_, err := p.db.Exec(query, args...)
	return err
}

func (p *PostgresAdapter) QueryRow(query string, args ...any) storage.Scanner {
	return &errScanner{s: p.db.QueryRow(query, args...)}
}

func (p *PostgresAdapter) Query(query string, args ...any) (storage.Rows, error) {
	return p.db.Query(query, args...)
}

func (p *PostgresAdapter) Close() error {
	return p.db.Close()
}

func (p *PostgresAdapter) TableColumns(table string) ([]string, error) {
	return tableColumns(p, table)
}

type errScanner struct{ s *sql.Row }

func (e errScanner) Scan(dest ...any) error {
	err := e.s.Scan(dest...)
	if err == sql.ErrNoRows {
		return storage.ErrNoRows
	}
	return err
}

var (
	_ storage.Conn          = (*PostgresAdapter)(nil)
	_ ddl.Compiler          = (*PostgresAdapter)(nil)
	_ ddl.TableIntrospector = (*PostgresAdapter)(nil)
)
```

> `tableColumns(q interface{ Query(string, ...any) (orm.Rows, error) }, table string)` en
> `adapter.go:83` solo cambia el tipo de retorno de la interfaz anónima: `(storage.Rows, error)`.

### 3.3 `translate.go` — separar DML de DDL, mismo patrón que `sqlt`

```go
package postgres

import (
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
	"github.com/tinywasm/storage"
)

// translate converts a storage.Query (DML only) into Postgres SQL. Body unchanged from today's
// switch, minus the DDL cases (moved to translateDDL below) — same shape as
// sqlt/docs/PLAN.md §3.3, adjust $1/$2 placeholder numbering exactly as it works today.
func translate(q storage.Query, m model.Model) (string, []any, error) {
	switch q.Action {
	case storage.ActionCreate:
		// ... cuerpo sin cambios de translate.go actual, case orm.ActionCreate ...
	case storage.ActionReadOne, storage.ActionReadAll:
		// ... cuerpo sin cambios ...
	case storage.ActionUpdate:
		// ... cuerpo sin cambios ...
	case storage.ActionDelete:
		// ... cuerpo sin cambios ...
	default:
		return "", nil, fmt.Errf("postgres: unknown DML action: %v", q.Action)
	}
}

// translateDDL converts a ddl.Stmt into Postgres SQL — the cases translate() used to have for
// ActionCreateTable/DropTable/AddColumn/RenameColumn/DropColumn, unchanged in SQL logic, moved
// here and re-keyed on ddl.Op / ddl.Stmt fields (Stmt.ColumnName for DropColumn — see
// ddl/docs/PLAN.md §3.1, same DropColumn shape correction as sqlt).
func translateDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	switch s.Op {
	case ddl.OpCreateTable:
		// ... cuerpo de la antigua rama ActionCreateTable, leyendo s.Table/m.Schema() ...
	case ddl.OpDropTable:
		// ... s.Table ...
	case ddl.OpAddColumn:
		// ... s.Table, s.Column ...
	case ddl.OpRenameColumn:
		// ... s.Table, s.Column, s.OldName ...
	case ddl.OpDropColumn:
		// ... s.Table, s.ColumnName (antes era un slice Columns []string, ahora un string) ...
	default:
		return "", nil, fmt.Errf("postgres: unknown DDL op: %v", s.Op)
	}
}

// Translate is the public DML export (translate_test.go compares against it).
func Translate(q storage.Query, m model.Model) (string, []any, error) {
	return translate(q, m)
}

// TranslateDDL is the new DDL counterpart.
func TranslateDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	return translateDDL(s, m)
}

func buildConditions(sb *fmt.Conv, conditions []storage.Condition, args *[]any, argIndex *int) error {
	// ... cuerpo sin cambios, solo el tipo del parámetro conditions ...
}
```

> **No reescribas la lógica SQL.** Copia cada `case` tal cual está hoy en `translate.go:47-250`,
> solo reubicándolo en `translate` o `translateDDL` según sea DML o DDL, y ajustando de dónde lee los
> campos (`q.X` → `s.X` para los casos DDL, con `s.ColumnName` reemplazando el viejo
> `q.Columns[0]`/slice en `DropColumn`).

### 3.4 `compiler.go` — el segundo tipo `Compiler` (usado hoy solo para `ExportDDL`)

```go
package postgres

import (
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/ddlc"
	"github.com/tinywasm/model"
)

type Compiler struct{}

func NewCompiler() *Compiler { return &Compiler{} }

func (c *Compiler) CompileDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	return translateDDL(s, m)
}

func (c *Compiler) ExportDDL(models []model.Model) (string, error) {
	// ... cuerpo sin cambios de lógica, salvo que ahora llama c.CompileDDL(ddl.Stmt{Op:
	// ddl.OpCreateTable, Table: m.ModelName()}, m) en vez de un Compile(orm.Query{Action:
	// orm.ActionCreateTable}) — mismo ajuste que sqlt/docs/PLAN.md §3.2.
}

var (
	_ ddl.Compiler  = (*Compiler)(nil)
	_ ddlc.Exporter = (*Compiler)(nil)
)
```

### 3.5 `introspect.go` — `orm.ColumnInfo` → `ddl.ColumnInfo`

Mismo cambio mecánico que `sqlite/docs/PLAN.md` §2.5: `columns(q querier, table string) ([]orm.ColumnInfo, error)` → `([]ddl.ColumnInfo, error)`; `PostgresAdapter.Columns`/`PostgresTx.Columns` idem; `var _ orm.SchemaInspector` → `var _ ddl.SchemaInspector`.

### 3.6 `tx.go` — `PostgresTx`, el equivalente transaccional

Mismo tratamiento que `PostgresAdapter`: `Compile`→`storage.Query`/`storage.Plan`, añade `CompileDDL` (delega a
`translateDDL`, igual que §3.2), `Exec`/`QueryRow`/`Query` re-tipados a `storage.Scanner`/`storage.Rows`,
`TableColumns`/`Tables`/`Columns` re-tipados a `ddl.TableIntrospector`/`ddl.SchemaInspector`.
`PostgresAdapter.BeginTx() (orm.TxBoundExecutor, error)` (`tx.go:79`) → `(storage.TxBoundExecutor, error)`.

### 3.7 `tests/conformance_test.go` (`package tests`) — ambas suites, sobre `storage`/`ddl`

```go
package tests

import (
	"database/sql"
	"os"
	"testing"

	"github.com/tinywasm/ddl"
	ddlconf "github.com/tinywasm/ddl/conformance"
	"github.com/tinywasm/model"
	"github.com/tinywasm/postgres"
	"github.com/tinywasm/storage"
	dbconf "github.com/tinywasm/storage/conformance"
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
				_, _ = raw.Exec("DROP TABLE IF EXISTS " + m.ModelName())
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

// DDL: drive tinywasm/ddl runtime with postgres's ddl.Compiler (PostgresAdapter itself).
func TestPostgres_DDLConformance(t *testing.T) {
	dsn := dsnOrSkip(t)
	ddlconf.Run(t, ddlconf.Factory{
		Name: "postgres",
		New: func(t *testing.T) (schema *ddl.DB, conn storage.Conn, cols func(string) []string) {
			raw, err := sql.Open("postgres", dsn)
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			_, _ = raw.Exec("DROP TABLE IF EXISTS conformance_widget")
			adapter := postgres.AdapterForTest(raw)
			schema = ddl.New(adapter, adapter) // PostgresAdapter is both storage.Conn and ddl.Compiler
			cols = func(table string) []string { /* information_schema.columns → names */ return nil }
			return schema, adapter, cols
		},
	})
}
```

> Ajusta `cols` a `SELECT column_name FROM information_schema.columns WHERE table_name=$1 ORDER BY
> ordinal_position`. `ddl.New(adapter, adapter)` funciona porque `*PostgresAdapter` implementa **tanto**
> `storage.Conn` como `ddl.Compiler` en el mismo valor (§3.2) — no necesitas el segundo tipo `Compiler`
> (§3.4) para este test, aunque `Compiler` sigue existiendo para `ExportDDL`.

## 4. Si alguna suite se pone en rojo → corregir `translate.go`/`adapter.go`

Nunca la suite. Puntos: placeholders `$1,$2` bien numerados, `sql.ErrNoRows`→`storage.ErrNoRows` en el
scanner, DDL de tipos Postgres válidos, `IN ($1,$2)`, booleanos, `ALTER TABLE ADD COLUMN`,
`buildDropColumn`/rama `OpDropColumn` leyendo `s.ColumnName` (no un slice) tras el cambio de §3.3.

## 5. Criterios de aceptación

- `*PostgresAdapter` implementa `storage.Conn` **y** `ddl.Compiler` **y** `ddl.TableIntrospector` **y**
  `ddl.SchemaInspector` (`var _` de los cuatro). **Cero** `github.com/tinywasm/orm`
  (`grep -rn "tinywasm/orm" .` vacío).
- `postgres.Open(dsn) (storage.Conn, error)` — sin `init()`, sin registro.
- Con `POSTGRES_DSN` alcanzable: `TestPostgres_DBConformance` y `TestPostgres_DDLConformance` verdes.
- Sin `POSTGRES_DSN`: ambos **saltan** limpio.
- `go.mod` en `storage@v0.0.1`+, `ddl@v0.0.1`+; `go mod tidy` limpio; publicado con `gopush`.

## 6. Etapas

| # | Etapa | Archivo(s) | Criterio |
|---|---|---|---|
| 1 | Bump deps, quitar orm | `go.mod` | `storage`/`ddl` añadidos; `orm` fuera |
| 2 | `Open` sin registro, `storage.Conn`+`ddl.Compiler` | `adapter.go` | `var _ storage.Conn`, `var _ ddl.Compiler` (§3.2) |
| 3 | Switch DML/DDL separados | `translate.go` | `translate`/`translateDDL` (§3.3) |
| 4 | `Compiler` (ExportDDL) | `compiler.go` | `CompileDDL` en vez de `Compile` (§3.4) |
| 5 | Introspección | `introspect.go` | `ddl.ColumnInfo`/`ddl.SchemaInspector` (§3.5) |
| 6 | `PostgresTx` | `tx.go` | mismo tratamiento que `PostgresAdapter` (§3.6) |
| 7 | Test DML | `tests/conformance_test.go` | `dbconf.Run` + skip (§3.7) |
| 8 | Test DDL | `tests/conformance_test.go` | `ddlconf.Run` + skip (§3.7) |
| 9 | Correcciones (si aplica) | `translate.go`/`adapter.go` | suites verdes contra Postgres real |
| 10 | Publicar | — | `gotest` verde; `gopush 'refactor!: storage+ddl conformance'` |

## 7. Cierre

Tras `gopush`, **borra** `docs/PLAN.md`; correcciones de comportamiento → `README.md`.
