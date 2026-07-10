package postgres

import "github.com/tinywasm/model"

import (
	"github.com/tinywasm/fmt"
	"github.com/tinywasm/orm"
	"github.com/tinywasm/ddlc"
)

// translate converts an ORM query to a PostgreSQL query and arguments.
func postgresType(t model.FieldType) string {
	switch t {
	case model.FieldInt:
		return "BIGINT"
	case model.FieldFloat:
		return "DOUBLE PRECISION"
	case model.FieldBool:
		return "BOOLEAN"
	case model.FieldBlob:
		return "BYTEA"
	default:
		return "TEXT"
	}
}

func postgresColumnType(f model.Field) string {
	if f.Type.Storage() == model.FieldText && f.Permitted.Maximum > 0 {
		return fmt.Sprintf("VARCHAR(%d)", f.Permitted.Maximum)
	}
	return postgresType(f.Type.Storage())
}

func onDeleteSQL(action string) string {
	switch action {
	case "restrict":
		return "RESTRICT"
	case "set_null":
		return "SET NULL"
	case "no_action":
		return "NO ACTION"
	default:
		return "CASCADE"
	}
}

func translate(q orm.Query, m model.Model) (string, []any, error) {
	sb := fmt.Convert()
	var args []any
	argIndex := 1

	switch q.Action {
	case orm.ActionCreate:
		sb.Write("INSERT INTO ")
		sb.Write(q.Table)
		sb.Write(" (")
		sb.Write(fmt.Convert(q.Columns).Join(", ").String())
		sb.Write(") VALUES (")
		for i, v := range q.Values {
			if i > 0 {
				sb.Write(", ")
			}
			sb.Write(fmt.Sprintf("$%d", argIndex))
			args = append(args, v)
			argIndex++
		}
		sb.Write(")")
		// Append RETURNING id if it's likely expected, although not strictly in generic ORM spec.
		// However, many ORMs rely on LastInsertId which Postgres doesn't support via Result.
		// So we often need `RETURNING id`.
		// Let's assume the user model has an 'id' column for now or handle it via Execute scan.
		// If we don't add it here, `Execute` won't get it back.
		// But generic `translate` shouldn't assume column names unless specified.
		// The `orm` might handle ID assignment via UUIDs generated in Go, in which case RETURNING isn't needed.
		// If the DB generates IDs (SERIAL/IDENTITY), we need it.
		// Let's stick to standard INSERT for now. If tests fail on ID retrieval, we'll revisit.

	case orm.ActionReadOne, orm.ActionReadAll:
		sb.Write("SELECT ")
		if len(q.Columns) == 0 {
			sb.Write("*")
		} else {
			sb.Write(fmt.Convert(q.Columns).Join(", ").String())
		}
		sb.Write(" FROM ")
		sb.Write(q.Table)
		if err := buildConditions(sb, q.Conditions, &args, &argIndex); err != nil {
			return "", nil, err
		}
		if len(q.OrderBy) > 0 {
			sb.Write(" ORDER BY ")
			for i, o := range q.OrderBy {
				if i > 0 {
					sb.Write(", ")
				}
				sb.Write(o.Column()) // Changed from o.Field to o.Column()
				sb.Write(" ")
				sb.Write(o.Dir()) // Changed from checking o.Desc to o.Dir() which returns "ASC" or "DESC"
			}
		}
		if q.Limit > 0 {
			sb.Write(fmt.Sprintf(" LIMIT %d", q.Limit))
		}
		if q.Offset > 0 {
			sb.Write(fmt.Sprintf(" OFFSET %d", q.Offset))
		}

	case orm.ActionUpdate:
		sb.Write("UPDATE ")
		sb.Write(q.Table)
		sb.Write(" SET ")
		for i, c := range q.Columns {
			if i > 0 {
				sb.Write(", ")
			}
			sb.Write(c)
			sb.Write(fmt.Sprintf(" = $%d", argIndex))
			args = append(args, q.Values[i])
			argIndex++
		}
		if err := buildConditions(sb, q.Conditions, &args, &argIndex); err != nil {
			return "", nil, err
		}

	case orm.ActionDelete:
		sb.Write("DELETE FROM ")
		sb.Write(q.Table)
		if err := buildConditions(sb, q.Conditions, &args, &argIndex); err != nil {
			return "", nil, err
		}

	case orm.ActionCreateTable:
		sb.Write("CREATE TABLE IF NOT EXISTS ")
		sb.Write(q.Table)
		sb.Write(" (")
		fields := m.Schema()

		// Count composite PK fields upfront to decide between inline and table-level PK.
		var pkCols []string
		for _, f := range fields {
			if f.IsPK() {
				pkCols = append(pkCols, f.Name)
			}
		}
		compositePK := len(pkCols) > 1

		for i, f := range fields {
			if i > 0 {
				sb.Write(", ")
			}
			sb.Write(f.Name)
			sb.Write(" ")
			isPK := f.IsPK()
			isAuto := f.IsAutoInc()
			if isPK && isAuto && !compositePK {
				if f.Type.Storage() == model.FieldInt {
					sb.Write("BIGSERIAL")
				} else {
					sb.Write("SERIAL")
				}
			} else if isAuto && f.Type.Storage() == model.FieldInt {
				sb.Write("BIGSERIAL")
			} else if isAuto {
				sb.Write("SERIAL")
			} else {
				sb.Write(postgresColumnType(f))
			}
			if isPK {
				if compositePK {
					// Composite PK: columns must be NOT NULL; constraint emitted as table-level below.
					sb.Write(" NOT NULL")
				} else {
					sb.Write(" PRIMARY KEY")
				}
			}
			if f.NotNull {
				sb.Write(" NOT NULL")
			}
			if f.IsUnique() {
				sb.Write(" UNIQUE")
			}
		}
		if compositePK {
			sb.Write(fmt.Sprintf(", PRIMARY KEY (%s)", fmt.Convert(pkCols).Join(", ").String()))
		}
		// ddlc.FieldExt is used for FKs. Since m.Schema() returns []model.Field,
		// we need to check if the implementation provided FieldExt.
		// In tinywasm/orm, models that have FKs can optionally implement an extended schema.
		if ext, ok := m.(interface{ SchemaExt() []ddlc.FieldExt }); ok {
			for _, f := range ext.SchemaExt() {
				if f.Ref != "" {
					refCol := f.RefColumn
					if refCol == "" {
						refCol = "id"
					}
					sb.Write(fmt.Sprintf(", CONSTRAINT fk_%s_%s FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s",
						q.Table, f.Name, f.Name, f.Ref, refCol, onDeleteSQL(f.OnDelete)))
				}
			}
		}

		sb.Write(")")

	case orm.ActionDropTable:
		sb.Write("DROP TABLE IF EXISTS ")
		sb.Write(q.Table)

	case orm.ActionAddColumn:
		if q.Column == nil || q.Table == "" {
			return "", nil, fmt.Err("table and column required for add column")
		}
		sb.Write("ALTER TABLE ")
		sb.Write(q.Table)
		sb.Write(" ADD COLUMN IF NOT EXISTS ")
		sb.Write(q.Column.Name)
		sb.Write(" ")
		sb.Write(postgresType(q.Column.Type.Storage()))

	case orm.ActionRenameColumn:
		if q.Column == nil || q.OldName == "" || q.Table == "" {
			return "", nil, fmt.Err("table, old name and column required for rename")
		}
		sb.Write("ALTER TABLE ")
		sb.Write(q.Table)
		sb.Write(" RENAME COLUMN ")
		sb.Write(q.OldName)
		sb.Write(" TO ")
		sb.Write(q.Column.Name)

	case orm.ActionDropColumn:
		if q.Table == "" || len(q.Columns) == 0 {
			return "", nil, fmt.Err("table and column required for drop column")
		}
		sb.Write("ALTER TABLE ")
		sb.Write(q.Table)
		sb.Write(" DROP COLUMN IF EXISTS ")
		sb.Write(q.Columns[0])

	case orm.ActionCreateDatabase:
		sb.Write("CREATE DATABASE ")
		sb.Write(q.Database)

	default:
		return "", nil, fmt.Errf("unsupported action: %d", q.Action)
	}

	return sb.String(), args, nil
}

// Translate exposes translate for external testing packages.
func Translate(q orm.Query, m model.Model) (string, []any, error) {
	return translate(q, m)
}

func buildConditions(sb *fmt.Conv, conditions []orm.Condition, args *[]any, argIndex *int) error {
	if len(conditions) == 0 {
		return nil
	}

	sb.Write(" WHERE ")
	for i, c := range conditions {
		if i > 0 {
			logic := c.Logic()
			if logic == "" {
				logic = "AND"
			}
			sb.Write(fmt.Sprintf(" %s ", logic))
		}

		op := c.Operator()
		if op == "IS NULL" || op == "IS NOT NULL" {
			sb.Write(c.Field())
			sb.Write(" ")
			sb.Write(op)
			continue
		}

		if op == "IN" {
			slice, ok := c.Value().([]any)
			if !ok {
				return fmt.Errf("IN operator requires []any value, got %T", c.Value())
			}
			if len(slice) == 0 {
				return fmt.Err("IN operator slice cannot be empty")
			}
			sb.Write(c.Field())
			sb.Write(" IN (")
			for j, val := range slice {
				if j > 0 {
					sb.Write(", ")
				}
				sb.Write(fmt.Sprintf("$%d", *argIndex))
				*args = append(*args, val)
				(*argIndex)++
			}
			sb.Write(")")
		} else {
			sb.Write(c.Field())
			sb.Write(" ")
			sb.Write(c.Operator())
			sb.Write(" ")
			sb.Write(fmt.Sprintf("$%d", *argIndex))
			*args = append(*args, c.Value())
			(*argIndex)++
		}
	}
	return nil
}
