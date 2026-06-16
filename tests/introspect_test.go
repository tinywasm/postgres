package tests

import (
	"os"
	"testing"

	"github.com/tinywasm/orm"
	"github.com/tinywasm/postgres"
)

func TestIntrospection(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:password@localhost:5432/postgres?sslmode=disable"
	}

	db, err := postgres.New(dsn)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	executor := db.RawExecutor().(orm.Executor)

	// 1. Setup a test table
	err = executor.Exec("DROP TABLE IF EXISTS test_introspect")
	if err != nil {
		t.Fatalf("failed to drop table: %v", err)
	}

	err = executor.Exec(`
		CREATE TABLE test_introspect (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			age INT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	defer executor.Exec("DROP TABLE test_introspect")

	// 2. Test Tables()
	adapter := db.RawExecutor().(orm.SchemaInspector)
	tables, err := adapter.Tables()
	if err != nil {
		t.Fatalf("failed to get tables: %v", err)
	}

	found := false
	for _, table := range tables {
		if table == "test_introspect" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("table 'test_introspect' not found in %v", tables)
	}

	// 3. Test Columns()
	cols, err := adapter.Columns("test_introspect")
	if err != nil {
		t.Fatalf("failed to get columns: %v", err)
	}

	expectedCols := map[string]struct {
		Type    string
		NotNull bool
		PK      bool
	}{
		"id":         {"integer", true, true},
		"name":       {"text", true, false},
		"age":        {"integer", false, false},
		"created_at": {"timestamp without time zone", false, false},
	}

	if len(cols) != len(expectedCols) {
		t.Errorf("expected %d columns, got %d", len(expectedCols), len(cols))
	}

	for _, col := range cols {
		expected, ok := expectedCols[col.Name]
		if !ok {
			t.Errorf("unexpected column: %s", col.Name)
			continue
		}
		if col.Type != expected.Type {
			t.Errorf("column %s: expected type %s, got %s", col.Name, expected.Type, col.Type)
		}
		if col.NotNull != expected.NotNull {
			t.Errorf("column %s: expected not null %v, got %v", col.Name, expected.NotNull, col.NotNull)
		}
		if col.PK != expected.PK {
			t.Errorf("column %s: expected PK %v, got %v", col.Name, expected.PK, col.PK)
		}
	}

	// 4. Test within Transaction
	err = db.Tx(func(tx *orm.DB) error {
		txInspector := tx.RawExecutor().(orm.SchemaInspector)

		// Test Tables within Tx
		tables, err := txInspector.Tables()
		if err != nil {
			return err
		}
		found := false
		for _, table := range tables {
			if table == "test_introspect" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("table 'test_introspect' not found in transaction")
		}

		// Test Columns within Tx
		cols, err := txInspector.Columns("test_introspect")
		if err != nil {
			return err
		}
		if len(cols) != 4 {
			t.Errorf("expected 4 columns in transaction, got %d", len(cols))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
}
