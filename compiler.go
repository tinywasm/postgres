package postgres

import (
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/ddlc"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
)

// Compiler implements the ddl.Compiler and ddlc.Exporter interfaces for PostgreSQL.
type Compiler struct{}

// NewCompiler creates a new Compiler instance.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// CompileDDL compiles a DDL statement.
func (c *Compiler) CompileDDL(s ddl.Stmt, m model.Model) (string, []any, error) {
	return translateDDL(s, m)
}

// ExportDDL generates a full DDL string for the given models.
func (c *Compiler) ExportDDL(models []model.Model) (string, error) {
	sorted, err := ddlc.TopologicalSort(models)
	if err != nil {
		return "", err
	}

	var buf fmt.Builder
	buf.Write("-- dialect: postgres\n\n")

	for _, m := range sorted {
		stmt := ddl.Stmt{
			Op:    ddl.OpCreateTable,
			Table: m.ModelName(),
		}
		query, _, err := c.CompileDDL(stmt, m)
		if err != nil {
			return "", err
		}
		buf.Write(query)
		buf.Write(";\n\n")

		// Auto-index on FK columns
		if ext, ok := m.(interface{ SchemaExt() []ddlc.FieldExt }); ok {
			for _, f := range ext.SchemaExt() {
				if f.Ref != "" {
					buf.Write(fmt.Sprintf(
						"CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);\n\n",
						m.ModelName(), f.Name, m.ModelName(), f.Name))
				}
			}
		}
	}

	return buf.String(), nil
}

// Ensure Compiler implements ddl.Compiler and ddlc.Exporter.
var _ ddl.Compiler = (*Compiler)(nil)
var _ ddlc.Exporter = (*Compiler)(nil)
