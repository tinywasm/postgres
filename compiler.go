package postgres

import (
	"github.com/tinywasm/ddl"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/model"
)

// Compiler implements ddl.Compiler and exposes ExportDDL for PostgreSQL DDL generation tooling.
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
	sorted, err := ddl.TopologicalSort(models)
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
		if ext, ok := m.(interface{ SchemaExt() []model.FieldExt }); ok {
			for _, f := range ext.SchemaExt() {
				if f.Ref != "" {
					buf.Write(fmt.Sprintf(
						"CREATE INDEX IF NOT EXISTS %s ON %s(%s);\n\n",
						quoteIdent(fmt.Sprintf("idx_%s_%s", m.ModelName(), f.Name)), quoteIdent(m.ModelName()), quoteIdent(f.Name)))
				}
			}
		}
	}

	return buf.String(), nil
}

// Ensure Compiler implements ddl.Compiler.
var _ ddl.Compiler = (*Compiler)(nil)
