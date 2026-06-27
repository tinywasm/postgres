//go:build !wasm

package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/orm/ddl"
	"github.com/tinywasm/postgres"
)

type userModel struct{}

func (m *userModel) ModelName() string { return "users" }
func (m *userModel) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true, AutoInc: true}},
		{Name: "username", Type: fmt.FieldText, NotNull: true, Permitted: fmt.Permitted{Maximum: 50}, DB: &fmt.FieldDB{Unique: true}},
		{Name: "email", Type: fmt.FieldText, NotNull: true, DB: &fmt.FieldDB{Unique: true}},
		{Name: "score", Type: fmt.FieldFloat},
		{Name: "active", Type: fmt.FieldBool},
		{Name: "avatar", Type: fmt.FieldBlob},
	}
}
func (m *userModel) Pointers() []any { return nil }

type roleModel struct{}

func (m *roleModel) ModelName() string { return "roles" }
func (m *roleModel) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true, AutoInc: true}},
		{Name: "name", Type: fmt.FieldText, NotNull: true, Permitted: fmt.Permitted{Maximum: 100}, DB: &fmt.FieldDB{Unique: true}},
	}
}
func (m *roleModel) Pointers() []any { return nil }

type sessionModel struct{}

func (m *sessionModel) ModelName() string { return "sessions" }
func (m *sessionModel) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "id", Type: fmt.FieldText, DB: &fmt.FieldDB{PK: true}},
		{Name: "user_id", Type: fmt.FieldInt},
		{Name: "metadata", Type: fmt.FieldText},
	}
}
func (m *sessionModel) SchemaExt() []orm.FieldExt {
	return []orm.FieldExt{
		{Field: fmt.Field{Name: "user_id"}, Ref: "users"},
	}
}
func (m *sessionModel) Pointers() []any { return nil }

type userRoleModel struct{}

func (m *userRoleModel) ModelName() string { return "user_roles" }
func (m *userRoleModel) Schema() []fmt.Field {
	return []fmt.Field{
		{Name: "user_id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true}},
		{Name: "role_id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true}},
	}
}
func (m *userRoleModel) SchemaExt() []orm.FieldExt {
	return []orm.FieldExt{
		{Field: fmt.Field{Name: "user_id"}, Ref: "users"},
		{Field: fmt.Field{Name: "role_id"}, Ref: "roles"},
	}
}
func (m *userRoleModel) Pointers() []any { return nil }

func TestExportDDL_FullSchema(t *testing.T) {
	goldenRaw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	// Remove header comments from golden file for comparison
	lines := strings.Split(string(goldenRaw), "\n")
	var sb strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "CREATE") || strings.HasPrefix(line, "-- dialect:") {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	golden := sb.String()

	c := postgres.NewCompiler()
	models := []fmt.Model{
		&userModel{},
		&roleModel{},
		&sessionModel{},
		&userRoleModel{},
	}

	got, err := c.ExportDDL(models)
	if err != nil {
		t.Fatalf("ExportDDL failed: %v", err)
	}

	gotClean := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "CREATE") || strings.HasPrefix(line, "-- dialect:") {
			gotClean += line + "\n"
		}
	}

	if strings.TrimSpace(gotClean) != strings.TrimSpace(golden) {
		t.Errorf("DDL output mismatch.\nGOT:\n%s\n\nEXPECTED:\n%s", gotClean, golden)
	}
}

func TestExportDDL_EmptyInput(t *testing.T) {
	c := postgres.NewCompiler()
	got, err := c.ExportDDL(nil)
	if err != nil {
		t.Fatalf("ExportDDL failed: %v", err)
	}
	expected := "-- dialect: postgres\n\n"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestExportDDL_ImplementsInterface(t *testing.T) {
	var _ ddl.Exporter = postgres.NewCompiler()
}
