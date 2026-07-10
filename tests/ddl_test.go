package tests

import "github.com/tinywasm/model"

import (
	"os"
	"testing"

	"github.com/tinywasm/ddlc"
	"github.com/tinywasm/postgres"
)

type MinimalModel struct {
	ID   int64  `db:"pk,autoincrement"`
	Name string `db:"unique,not_null"`
}

func (m *MinimalModel) ModelName() string {
	return "minimal_models"
}

func (m *MinimalModel) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "name", Type: model.Text(), NotNull: true, DB: &model.FieldDB{Unique: true}},
	}
}

func (m *MinimalModel) Pointers() []any {
	return []any{&m.ID, &m.Name}
}

func NewMinimalModel() model.Model {
	return &MinimalModel{}
}

type RelatedModel struct {
	ID        int64
	MinimalID int64
}

func (r *RelatedModel) ModelName() string {
	return "related_models"
}

func (r *RelatedModel) Schema() []model.Field {
	return []model.Field{
		{Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
		{Name: "minimal_id", Type: model.Int()},
	}
}

func (r *RelatedModel) SchemaExt() []ddlc.FieldExt {
	return []ddlc.FieldExt{
		{Field: model.Field{Name: "minimal_id"}, Ref: "minimal_models", RefColumn: "id"},
	}
}

func (r *RelatedModel) Pointers() []any {
	return []any{&r.ID, &r.MinimalID}
}

func NewRelatedModel() model.Model {
	return &RelatedModel{}
}

func TestDDL(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}

	dbORM, err := postgres.New(dsn)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer dbORM.Close()

	// Ensure clean state before tests
	_ = dbORM.DropTable(&RelatedModel{})
	_ = dbORM.DropTable(&MinimalModel{})

	// Test CreateTable IF NOT EXISTS (MinimalModel)
	if err := dbORM.CreateTable(&MinimalModel{}); err != nil {
		t.Fatalf("Failed to create MinimalModel table: %v", err)
	}

	// Should not fail if executed again
	if err := dbORM.CreateTable(&MinimalModel{}); err != nil {
		t.Fatalf("Failed to create MinimalModel table IF NOT EXISTS: %v", err)
	}

	// Test CreateTable with FK (RelatedModel)
	if err := dbORM.CreateTable(&RelatedModel{}); err != nil {
		t.Fatalf("Failed to create RelatedModel table with FK: %v", err)
	}

	// Insert data to verify table creation constraints
	minimal1 := &MinimalModel{Name: "Test 1"}
	if err := dbORM.Create(minimal1); err != nil {
		t.Fatalf("Failed to insert into MinimalModel: %v", err)
	}

	// Check Not Null / Unique constraints
	minimalDuplicate := &MinimalModel{Name: "Test 1"}
	if err := dbORM.Create(minimalDuplicate); err == nil {
		t.Fatalf("Expected unique constraint violation")
	}

	// Insert into related to check FK
	badRelated := &RelatedModel{MinimalID: 9999}
	if err := dbORM.Create(badRelated); err == nil {
		t.Fatalf("Expected FK constraint violation")
	}

	// Clean up tables
	if err := dbORM.DropTable(&RelatedModel{}); err != nil {
		t.Fatalf("Failed to drop RelatedModel table: %v", err)
	}
	if err := dbORM.DropTable(&MinimalModel{}); err != nil {
		t.Fatalf("Failed to drop MinimalModel table: %v", err)
	}

	// Should not fail if dropped again
	if err := dbORM.DropTable(&MinimalModel{}); err != nil {
		t.Fatalf("Failed to drop MinimalModel table IF EXISTS: %v", err)
	}
}
