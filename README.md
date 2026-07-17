# PostgreSQL Adapter for tinywasm/storage + ddl
<img src="docs/img/badges.svg">

This repository implements the `storage.Conn` (from `github.com/tinywasm/storage`) and `ddl.Compiler` (from `github.com/tinywasm/ddl`) interfaces for PostgreSQL.

## Usage

Instead of a global registry or init-driven registration, the PostgreSQL adapter is constructed explicitly:

```go
package main

import (
	"log"

	"github.com/tinywasm/postgres"
)

func main() {
	dsn := "postgres://user:password@localhost:5432/dbname?sslmode=disable"

	// Open returns a storage.Conn
	conn, err := postgres.Open(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Use conn...
}
```

## Features

- Full `storage.Conn` implementation (unifying DML Compilation and Execution).
- Transaction support via `BeginTx` returning `storage.TxBoundExecutor`.
- Secure SQL generation with parameterized queries using `$1`, `$2`, etc.
- Support for `Create`, `ReadOne`, `ReadAll`, `Update`, `Delete` DML actions.
- Full DDL schema creation and compilation using `ddl.Compiler`.
- High-efficiency row scanning with `sql.ErrNoRows` cleanly mapped to `storage.ErrNoRows`.
- Complies with `storage/conformance` and `ddl/conformance` test suites.

## Documentation

- [Postgres Setup & Troubleshooting](docs/POSTGRES_SETUP.md)
