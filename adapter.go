package postgres

import (
	"database/sql"

	_ "github.com/lib/pq"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
)

func init() {
	orm.Register("postgres", New)
}

// PostgresAdapter is a PostgreSQL adapter for the tinywasm/orm library.
type PostgresAdapter struct {
	db *sql.DB
}

// Ensure PostgresAdapter implements orm.Compiler.
// We use a blank identifier assignment here so the Go compiler statically checks
// that *PostgresAdapter satisfies orm.Compiler. This does not create global state;
// since it assigns to `_` using `nil`, it consumes zero memory at runtime and only
// serves as a compile-time safeguard.
var _ orm.Compiler = (*PostgresAdapter)(nil)

// Ensure PostgresAdapter implements orm.Executor.
var _ orm.Executor = (*PostgresAdapter)(nil)

// New creates a new PostgresAdapter wrapped in an orm.DB.
// The dataSourceName parameter expects a valid PostgreSQL connection string.
// Example: "postgres://user:password@host:port/dbname?sslmode=disable"
func New(dataSourceName string) (*orm.DB, error) {
	db, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, err // This branch might be hard to hit because sql.Open mostly validates the driver name.
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	adapter := &PostgresAdapter{db: db}
	return orm.New(adapter, adapter), nil
}

// AdapterForTest returns the raw PostgresAdapter for testing purposes.
func AdapterForTest(db *sql.DB) *PostgresAdapter {
	return &PostgresAdapter{db: db}
}

// Compile compiles the ORM query into a Plan.
func (p *PostgresAdapter) Compile(q orm.Query, m fmt.Model) (orm.Plan, error) {
	query, args, err := translate(q, m)
	if err != nil {
		return orm.Plan{}, err
	}
	return orm.Plan{
		Query: query,
		Args:  args,
	}, nil
}

// Exec executes a query without returning result rows.
func (p *PostgresAdapter) Exec(query string, args ...any) error {
	_, err := p.db.Exec(query, args...)
	return err
}

// QueryRow executes a query that is expected to return at most one row.
func (p *PostgresAdapter) QueryRow(query string, args ...any) orm.Scanner {
	return errScanner{p.db.QueryRow(query, args...)}
}

// Query executes a query that returns rows.
func (p *PostgresAdapter) Query(query string, args ...any) (orm.Rows, error) {
	return p.db.Query(query, args...)
}

// Close closes the database connection.
func (p *PostgresAdapter) Close() error {
	return p.db.Close()
}

func tableColumns(q interface{ Query(string, ...any) (orm.Rows, error) }, table string) ([]string, error) {
	rows, err := q.Query(
		`SELECT column_name FROM information_schema.columns WHERE table_name = $1`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// TableColumns returns the column names for the given table.
func (p *PostgresAdapter) TableColumns(table string) ([]string, error) {
	return tableColumns(p, table)
}

type errScanner struct {
	s orm.Scanner
}

func (e errScanner) Scan(dest ...any) error {
	if err := e.s.Scan(dest...); err != nil {
		if err == sql.ErrNoRows {
			return orm.ErrNoRows
		}
		return err
	}
	return nil
}
