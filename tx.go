package postgres

import (
	"context"
	"database/sql"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
)

// PostgresTx wraps a SQL transaction.
type PostgresTx struct {
	tx      *sql.Tx
	adapter *PostgresAdapter
}

// Ensure PostgresTx implements orm.Compiler.
var _ orm.Compiler = (*PostgresTx)(nil)

// Ensure PostgresTx implements orm.Executor.
var _ orm.Executor = (*PostgresTx)(nil)

// Compile delegates to the PostgresAdapter.
func (p *PostgresTx) Compile(q orm.Query, m fmt.Model) (orm.Plan, error) {
	return p.adapter.Compile(q, m)
}

// Exec executes a query within the transaction.
func (p *PostgresTx) Exec(query string, args ...any) error {
	_, err := p.tx.Exec(query, args...)
	return err
}

// QueryRow executes a query that is expected to return at most one row within the transaction.
func (p *PostgresTx) QueryRow(query string, args ...any) orm.Scanner {
	return errScanner{p.tx.QueryRow(query, args...)}
}

// Query executes a query that returns rows within the transaction.
func (p *PostgresTx) Query(query string, args ...any) (orm.Rows, error) {
	return p.tx.Query(query, args...)
}

// Close is a no-op for generic Executor but here we could rollback if active.
// We let orm.DB handle transaction lifecycle.
func (p *PostgresTx) Close() error {
	return nil
}

// Commit commits the transaction.
func (p *PostgresTx) Commit() error {
	return p.tx.Commit()
}

// Rollback aborts the transaction.
func (p *PostgresTx) Rollback() error {
	return p.tx.Rollback()
}

// TableColumns returns the column names for the given table within the transaction.
func (p *PostgresTx) TableColumns(table string) ([]string, error) {
	return tableColumns(p, table)
}

// Tables returns all user-defined table names in the current schema.
func (p *PostgresTx) Tables() ([]string, error) {
	return tables(p)
}

// Columns returns full column metadata for the given table.
func (p *PostgresTx) Columns(table string) ([]orm.ColumnInfo, error) {
	return columns(p, table)
}

// Ensure PostgresTx implements orm.SchemaInspector
var _ orm.SchemaInspector = (*PostgresTx)(nil)

// BeginTx starts a transaction and returns a new orm.TxBoundExecutor.
func (p *PostgresAdapter) BeginTx() (orm.TxBoundExecutor, error) {
	tx, err := p.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return &PostgresTx{tx: tx, adapter: p}, nil
}
