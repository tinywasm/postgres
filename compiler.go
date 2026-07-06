package postgres

import (
	"github.com/tinywasm/model"
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/orm/ddl"
)

// Compiler implements the orm.Compiler and ddl.Exporter interfaces for PostgreSQL.
type Compiler struct{}

// NewCompiler creates a new Compiler instance.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// Compile compiles an ORM query into a Plan.
func (c *Compiler) Compile(q orm.Query, m model.Model) (orm.Plan, error) {
	query, args, err := Translate(q, m)
	if err != nil {
		return orm.Plan{}, err
	}
	return orm.Plan{
		Query: query,
		Args:  args,
	}, nil
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
		q := orm.Query{
			Action: orm.ActionCreateTable,
			Table:  m.ModelName(),
		}
		plan, err := c.Compile(q, m)
		if err != nil {
			return "", err
		}
		buf.Write(plan.Query)
		buf.Write(";\n\n")

		// Auto-index on FK columns
		if ext, ok := m.(interface{ SchemaExt() []orm.FieldExt }); ok {
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

// Ensure Compiler implements ddl.Exporter and orm.Compiler.
var _ ddl.Exporter = (*Compiler)(nil)
var _ orm.Compiler = (*Compiler)(nil)
