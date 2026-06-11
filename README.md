# PostgreSQL Adapter for tinywasm/orm
<img src="docs/img/badges.svg">

This repository implements the `orm.Adapter` interface for PostgreSQL, allowing it to be used with the `github.com/tinywasm/orm` library.

## Usage

```go
package main

import (
	"log"

	"github.com/tinywasm/postgres"
	"github.com/tinywasm/orm"
)

func main() {
	dsn := "postgres://user:password@localhost:5432/dbname?sslmode=disable"
	db, err := postgre.New(dsn)
	if err != nil {
		log.Fatal(err)
	}

	// Use db...
}
```

## Features

- Full `orm.Adapter` implementation.
- Transaction support via `BeginTx`.
- Secure SQL generation with parameterized queries using `$1`, `$2`, etc.
- Support for `Create`, `ReadOne`, `ReadAll`, `Update`, `Delete`.
- Efficient row scanning.
- **Schema-Sync support**: Registers as `"postgres"`, maps `ErrNoRows`, and implements `TableIntrospector` for full reconcile (additive column sync, column rename, and safe-drop).

## Update

`db.Update` always requires at least one `Condition`. This is enforced at
compile time by `tinywasm/orm`. There is no "update by PK implicitly" magic.

```go
// ✅ Correct
if err := db.Update(&user, orm.Eq(User_.ID, user.ID)); err != nil { ... }

// ❌ Compile error (caught by tinywasm/orm — will not reach the PostgreSQL layer)
db.Update(&user)
```

## Documentation

- [Postgres Setup & Troubleshooting](docs/POSTGRES_SETUP.md)
