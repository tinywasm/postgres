# Plan: FieldDB Compatibility

## Depends on

- github.com/tinywasm/fmt with FieldDB support

## Problem

translate.go accesses `f.PK`, `f.Unique`, `f.AutoInc` directly on `fmt.Field`. These fields moved to `fmt.FieldDB` struct behind `Field.DB *FieldDB` pointer.

## Changes

### 1. translate.go — use helper methods

| Line | Before | After |
|------|--------|-------|
| 118 | `if f.PK` | `if f.IsPK()` |
| 130 | `isPK := f.PK` | `isPK := f.IsPK()` |
| 131 | `isAuto := f.AutoInc` | `isAuto := f.IsAutoInc()` |
| 156 | `if f.Unique` | `if f.IsUnique()` |

### 2. Test schema literals

**tests/adapter_test.go**:
```go
// Before
{Name: "id", Type: fmt.FieldInt, PK: true, AutoInc: true}
{Name: "name", Type: fmt.FieldText, NotNull: true, Unique: true}

// After
{Name: "id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true, AutoInc: true}}
{Name: "name", Type: fmt.FieldText, NotNull: true, DB: &fmt.FieldDB{Unique: true}}
```

**tests/postgres_translate_test.go**:
```go
// Before
{Name: "id", Type: fmt.FieldText, PK: true}

// After
{Name: "id", Type: fmt.FieldText, DB: &fmt.FieldDB{PK: true}}
```

**tests/ddl_test.go**:
```go
// Before
{Name: "id", Type: fmt.FieldInt, PK: true, AutoInc: true}
{Name: "name", Type: fmt.FieldText, Unique: true, NotNull: true}

// After
{Name: "id", Type: fmt.FieldInt, DB: &fmt.FieldDB{PK: true, AutoInc: true}}
{Name: "name", Type: fmt.FieldText, DB: &fmt.FieldDB{Unique: true}, NotNull: true}
```

### 3. Bump go.mod

Update `github.com/tinywasm/fmt` to version with FieldDB.

## Execution Order

1. Bump fmt dependency
2. Update translate.go (4 lines)
3. Update tests/adapter_test.go (3 schema literals)
4. Update tests/postgres_translate_test.go (1 schema literal)
5. Update tests/ddl_test.go (3 schema literals)
6. `go test ./...`
