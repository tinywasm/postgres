# postgres — Implementar orm.SchemaInspector

Extender el `PostgresAdapter` para implementar `orm.SchemaInspector`, la
interfaz que permite al subpaquete `orm/mcp` registrar la tool `db_schema`.

**Prerequisito:** `tinywasm/orm` debe estar publicado con `orm.SchemaInspector`
y `orm.ColumnInfo` disponibles antes de aplicar este plan.

---

## Contexto

`adapter.go` ya implementa `orm.TableIntrospector` (`tableColumns` + método
`TableColumns` usando `information_schema.columns`). `orm.SchemaInspector` es
una interfaz separada más rica que agrega `Tables()` y `Columns()` con tipo
completo, NOT NULL y PK — necesaria para la tool MCP `db_schema`.

Los datos ya están disponibles en `information_schema`: la consulta existente
los lee parcialmente.

---

## Cambio requerido — nuevo archivo `postgres/introspect.go`

Separar la lógica de introspección de `adapter.go` en su propio archivo y
agregar `Tables()` + `Columns()` completos.

```go
package postgres

import "github.com/tinywasm/orm"

// Tables returns all user-defined table names in the current schema.
func (p *PostgresAdapter) Tables() ([]string, error) {
    rows, err := p.Query(`
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = 'public'
          AND table_type = 'BASE TABLE'
        ORDER BY table_name
    `)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var tables []string
    for rows.Next() {
        var name string
        if err := rows.Scan(&name); err != nil {
            return nil, err
        }
        tables = append(tables, name)
    }
    return tables, rows.Err()
}

// Columns returns full column metadata for the given table.
func (p *PostgresAdapter) Columns(table string) ([]orm.ColumnInfo, error) {
    rows, err := p.Query(`
        SELECT
            c.column_name,
            c.data_type,
            c.is_nullable = 'NO' AS not_null,
            COALESCE(
                (SELECT true
                 FROM information_schema.table_constraints tc
                 JOIN information_schema.key_column_usage kcu
                   ON tc.constraint_name = kcu.constraint_name
                  AND tc.table_name = kcu.table_name
                 WHERE tc.constraint_type = 'PRIMARY KEY'
                   AND kcu.table_name = c.table_name
                   AND kcu.column_name = c.column_name
                 LIMIT 1),
                false
            ) AS is_pk
        FROM information_schema.columns c
        WHERE c.table_schema = 'public'
          AND c.table_name = $1
        ORDER BY c.ordinal_position
    `, table)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var cols []orm.ColumnInfo
    for rows.Next() {
        var col orm.ColumnInfo
        var notNull, pk bool
        if err := rows.Scan(&col.Name, &col.Type, &notNull, &pk); err != nil {
            return nil, err
        }
        col.NotNull = notNull
        col.PK = pk
        cols = append(cols, col)
    }
    return cols, rows.Err()
}

// Ensure PostgresAdapter implements orm.SchemaInspector
var _ orm.SchemaInspector = (*PostgresAdapter)(nil)
```

---

## Archivos afectados

| Archivo | Cambio |
|---------|--------|
| `postgres/introspect.go` | Nuevo — `Tables()`, `Columns()`, compile-check |

> `tableColumns` en `adapter.go` (usado por `orm.TableIntrospector`) no se
> toca — sigue siendo responsable del sync del ORM.

---

## Orden de ejecución

1. Verificar que `github.com/tinywasm/orm` en `go.mod` tenga la versión con `SchemaInspector`
2. Crear `postgres/introspect.go` con `Tables()`, `Columns()`, compile-check
3. Publicar con `gopush`

---

## Verificación

```bash
gotest
```

El compile-check fallará si `orm.SchemaInspector` o `orm.ColumnInfo` no existen
— señal de que el prerequisito no está publicado.
