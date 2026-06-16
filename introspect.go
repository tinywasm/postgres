package postgres

import "github.com/tinywasm/orm"

type querier interface {
	Query(query string, args ...any) (orm.Rows, error)
}

func tables(q querier) ([]string, error) {
	rows, err := q.Query(`
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

func columns(q querier, table string) ([]orm.ColumnInfo, error) {
	rows, err := q.Query(`
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

// Tables returns all user-defined table names in the current schema.
func (p *PostgresAdapter) Tables() ([]string, error) {
	return tables(p)
}

// Columns returns full column metadata for the given table.
func (p *PostgresAdapter) Columns(table string) ([]orm.ColumnInfo, error) {
	return columns(p, table)
}

// Ensure PostgresAdapter implements orm.SchemaInspector
var _ orm.SchemaInspector = (*PostgresAdapter)(nil)
