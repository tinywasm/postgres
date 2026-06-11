//go:build !wasm

package tests

import (
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/postgres"
)

func TestTranslate_NewActions(t *testing.T) {
	m := &testUserModel{}

	t.Run("ActionAddColumn", func(t *testing.T) {
		q := orm.Query{
			Action: orm.ActionAddColumn,
			Table:  "users",
			Column: &fmt.Field{Name: "bio", Type: fmt.FieldText},
		}
		sql, _, err := postgres.Translate(q, m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT"
		if sql != expected {
			t.Errorf("expected %q, got %q", expected, sql)
		}
	})

	t.Run("ActionRenameColumn", func(t *testing.T) {
		q := orm.Query{
			Action:  orm.ActionRenameColumn,
			Table:   "users",
			OldName: "age",
			Column:  &fmt.Field{Name: "years", Type: fmt.FieldInt},
		}
		sql, _, err := postgres.Translate(q, m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ALTER TABLE users RENAME COLUMN age TO years"
		if sql != expected {
			t.Errorf("expected %q, got %q", expected, sql)
		}
	})

	t.Run("ActionDropColumn", func(t *testing.T) {
		q := orm.Query{
			Action:  orm.ActionDropColumn,
			Table:   "users",
			Columns: []string{"bio"},
		}
		sql, _, err := postgres.Translate(q, m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ALTER TABLE users DROP COLUMN IF EXISTS bio"
		if sql != expected {
			t.Errorf("expected %q, got %q", expected, sql)
		}
	})
}

func TestTranslate_NullConditions(t *testing.T) {
	m := &testUserModel{}

	t.Run("IsNotNull", func(t *testing.T) {
		q := orm.Query{
			Action:     orm.ActionReadAll,
			Table:      "users",
			Conditions: []orm.Condition{orm.IsNotNull("name")},
		}
		sql, args, err := postgres.Translate(q, m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !fmt.Contains(sql, "name IS NOT NULL") {
			t.Errorf("expected IS NOT NULL in SQL, got: %s", sql)
		}
		if len(args) != 0 {
			t.Errorf("expected 0 args, got %d", len(args))
		}
	})
}
