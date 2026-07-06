//go:build !wasm

package tests

import "github.com/tinywasm/model"

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
func (u *testUserModel) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.FieldText, DB: &model.FieldDB{PK: true}},
		{Name: "name", Type: model.FieldText},
		{Name: "age", Type: model.FieldInt},
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
		Values:  model.ReadValues(m.Schema(), m.Pointers()),
		// At least one condition — as guaranteed by tinywasm/orm after the fix.
		Conditions: []orm.Condition{orm.Eq("id", "abc123")},
	}

	sql, args, err := postgres.Translate(q, m)
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

type varcharModel struct{}

func (m *varcharModel) ModelName() string { return "varchars" }
func (m *varcharModel) Schema() []model.Field {
	return []model.Field{
		{Name: "username", Type: model.FieldText, Permitted: model.Permitted{Maximum: 100}},
		{Name: "email", Type: model.FieldText},
		{Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true, AutoInc: true}},
	}
}
func (m *varcharModel) Pointers() []any { return nil }

func TestVarchar_Postgres(t *testing.T) {
	m := &varcharModel{}
	q := orm.Query{Action: orm.ActionCreateTable, Table: m.ModelName()}
	sql, _, err := postgres.Translate(q, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fmt.Contains(sql, "username VARCHAR(100)") {
		t.Errorf("expected VARCHAR(100) for username, got: %s", sql)
	}
	if !fmt.Contains(sql, "email TEXT") {
		t.Errorf("expected TEXT for email, got: %s", sql)
	}
	if !fmt.Contains(sql, "id BIGSERIAL PRIMARY KEY") {
		t.Errorf("expected BIGSERIAL for id, got: %s", sql)
	}
}

type onDeleteModel struct{}

func (m *onDeleteModel) ModelName() string { return "on_deletes" }
func (m *onDeleteModel) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true}},
		{Name: "user_id", Type: model.FieldInt},
		{Name: "role_id", Type: model.FieldInt},
	}
}
func (m *onDeleteModel) SchemaExt() []orm.FieldExt {
	return []orm.FieldExt{
		{Field: model.Field{Name: "user_id"}, Ref: "users", OnDelete: "restrict"},
		{Field: model.Field{Name: "role_id"}, Ref: "roles"}, // Default CASCADE
	}
}
func (m *onDeleteModel) Pointers() []any { return nil }

func TestOnDelete_Postgres(t *testing.T) {
	m := &onDeleteModel{}
	q := orm.Query{Action: orm.ActionCreateTable, Table: m.ModelName()}
	sql, _, err := postgres.Translate(q, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fmt.Contains(sql, "CONSTRAINT fk_on_deletes_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT") {
		t.Errorf("expected ON DELETE RESTRICT for user_id, got: %s", sql)
	}
	if !fmt.Contains(sql, "CONSTRAINT fk_on_deletes_role_id FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE") {
		t.Errorf("expected ON DELETE CASCADE for role_id, got: %s", sql)
	}

	// Test other actions
	m2 := &onDeleteModel{}
	q2 := orm.Query{Action: orm.ActionCreateTable, Table: m2.ModelName()}

	cases := []struct {
		action   string
		expected string
	}{
		{"set_null", "SET NULL"},
		{"no_action", "NO ACTION"},
	}

	for _, c := range cases {
		m2Ext := []orm.FieldExt{
			{Field: model.Field{Name: "user_id"}, Ref: "users", OnDelete: c.action},
		}
		// We need a model that returns these SchemaExt
		sql, _, _ := postgres.Translate(q2, &mockOnDeleteModel{m2Ext})
		if !fmt.Contains(sql, "ON DELETE "+c.expected) {
			t.Errorf("expected ON DELETE %s for action %s, got: %s", c.expected, c.action, sql)
		}
	}
}

type mockOnDeleteModel struct {
	ext []orm.FieldExt
}

func (m *mockOnDeleteModel) ModelName() string { return "on_deletes" }
func (m *mockOnDeleteModel) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true}},
		{Name: "user_id", Type: model.FieldInt},
	}
}
func (m *mockOnDeleteModel) SchemaExt() []orm.FieldExt { return m.ext }
func (m *mockOnDeleteModel) Pointers() []any          { return nil }

// TestTranslate_Update_MultipleConditions verifies that AND conditions in an
// UPDATE query produce correct SQL.
func TestTranslate_Update_MultipleConditions(t *testing.T) {
	m := &testUserModel{ID: "abc123", Name: "Alice", Age: 30}
	q := orm.Query{
		Action:  orm.ActionUpdate,
		Table:   "users",
		Columns: []string{"id", "name", "age"},
		Values:  model.ReadValues(m.Schema(), m.Pointers()),
		Conditions: []orm.Condition{
			orm.Eq("id", "abc123"),
			orm.Eq("name", "Alice"),
		},
	}

	sql, args, err := postgres.Translate(q, m)
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
