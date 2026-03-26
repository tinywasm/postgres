//go:build !wasm

package postgres_test

import (
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/postgres"
)

// testUserModel is a minimal test model with a TEXT primary key (string PK, like unixid).
type testUserModel struct {
	ID   string
	Name string
	Age  int64
}

func (u *testUserModel) ModelName() string { return "users" }
func (u *testUserModel) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "id", Type: fmt.FieldText, PK: true},
		{Name: "name", Type: fmt.FieldText},
		{Name: "age", Type: fmt.FieldInt},
	}
}
func (u *testUserModel) Pointers() []any { return []any{&u.ID, &u.Name, &u.Age} }

// TestTranslate_Update_WithCondition verifies that translate() generates a
// valid UPDATE ... SET ... WHERE ... when at least one condition is present.
// This is the contract guaranteed by tinywasm/orm's mandatory first Condition.
func TestTranslate_Update_WithCondition(t *testing.T) {
	m := &testUserModel{ID: "abc123", Name: "Alice", Age: 30}
	q := orm.Query{
		Action:  orm.ActionUpdate,
		Table:   "users",
		Columns: []string{"id", "name", "age"},
		Values:  fmt.ReadValues(m.Schema(), m.Pointers()),
		// At least one condition — as guaranteed by tinywasm/orm after the fix.
		Conditions: []orm.Condition{orm.Eq("id", "abc123")},
	}

	sql, args, err := postgres.ExportTranslate(q, m)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Must contain WHERE clause.
	if !fmt.Contains(sql, "WHERE") {
		t.Errorf("expected WHERE clause in UPDATE, got: %s", sql)
	}

	// Must use parameterized form ($N for postgres).
	if !fmt.Contains(sql, "$") {
		t.Errorf("expected parameterized query, got: %s", sql)
	}

	// Condition value must appear in args.
	pkFound := false
	for _, a := range args {
		if a == "abc123" {
			pkFound = true
			break
		}
	}
	if !pkFound {
		t.Errorf("expected PK value 'abc123' in args, got: %v", args)
	}
}

// TestTranslate_Update_MultipleConditions verifies that AND conditions in an
// UPDATE query produce correct SQL.
func TestTranslate_Update_MultipleConditions(t *testing.T) {
	m := &testUserModel{ID: "abc123", Name: "Alice", Age: 30}
	q := orm.Query{
		Action:  orm.ActionUpdate,
		Table:   "users",
		Columns: []string{"id", "name", "age"},
		Values:  fmt.ReadValues(m.Schema(), m.Pointers()),
		Conditions: []orm.Condition{
			orm.Eq("id", "abc123"),
			orm.Eq("name", "Alice"),
		},
	}

	sql, args, err := postgres.ExportTranslate(q, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fmt.Contains(sql, "WHERE") {
		t.Errorf("expected WHERE in SQL, got: %s", sql)
	}
	if !fmt.Contains(sql, "AND") {
		t.Errorf("expected AND between conditions, got: %s", sql)
	}
	_ = args
}
