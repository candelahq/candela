package bigquery

import (
	"context"
	"fmt"
	"testing"

	bq "cloud.google.com/go/bigquery"
)

// schemaStore creates a Store that can call ensureSchema/evolveSchema.
func schemaStore(client *mockBQClient) *Store {
	return NewWithClient(client, Config{
		ProjectID: "test-project",
		Dataset:   "test_dataset",
		Table:     "spans",
		Location:  "US",
	})
}

// ── ensureSchema Tests ───────────────────────────────────────────────

func TestEnsureSchema_CreatesDatasetAndTable(t *testing.T) {
	client := newMockBQClient()
	table := &mockBQTable{id: "spans"}
	client.dataset.tables["spans"] = table

	s := schemaStore(client)
	err := s.ensureSchema(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !client.dataset.createCalled {
		t.Error("expected dataset.Create to be called")
	}
	if !table.createCalled {
		t.Error("expected table.Create to be called")
	}
}

func TestEnsureSchema_DatasetExists_CreatesTable(t *testing.T) {
	client := newMockBQClient()
	client.dataset.createErr = fmt.Errorf("googleapi: Error 409: Already Exists: Dataset")
	table := &mockBQTable{id: "spans"}
	client.dataset.tables["spans"] = table

	s := schemaStore(client)
	err := s.ensureSchema(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !client.dataset.createCalled {
		t.Error("expected dataset.Create to be called")
	}
	if !table.createCalled {
		t.Error("expected table.Create to be called")
	}
}

func TestEnsureSchema_TableExists_EvolvesSchema(t *testing.T) {
	client := newMockBQClient()
	client.dataset.createErr = fmt.Errorf("googleapi: Error 409: Already Exists: Dataset")

	// Table exists — return "Already Exists" on Create.
	// evolveSchema will be called; provide metadata with one column fewer
	// than the inferred schema.
	existingSchema := bq.Schema{
		{Name: "span_id", Type: bq.StringFieldType},
		{Name: "trace_id", Type: bq.StringFieldType},
	}
	table := &mockBQTable{
		id:        "spans",
		createErr: fmt.Errorf("googleapi: Error 409: Already Exists: Table"),
		metadataResult: &bq.TableMetadata{
			Schema: existingSchema,
			ETag:   "etag-1",
		},
	}
	client.dataset.tables["spans"] = table

	s := schemaStore(client)
	err := s.ensureSchema(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !table.updateCalled {
		t.Error("expected table.Update to be called for schema evolution")
	}
	// Updated schema should have MORE fields than existing.
	if len(table.updateSchema) <= len(existingSchema) {
		t.Errorf("updated schema should have more fields than existing: got %d, existing %d",
			len(table.updateSchema), len(existingSchema))
	}
}

func TestEnsureSchema_TableExists_NoSchemaChange(t *testing.T) {
	client := newMockBQClient()
	client.dataset.createErr = fmt.Errorf("googleapi: Error 409: Already Exists: Dataset")

	// Infer the full schema so it matches — no evolution needed.
	fullSchema, err := bq.InferSchema(spanRow{})
	if err != nil {
		t.Fatalf("failed to infer schema: %v", err)
	}

	table := &mockBQTable{
		id:        "spans",
		createErr: fmt.Errorf("googleapi: Error 409: Already Exists: Table"),
		metadataResult: &bq.TableMetadata{
			Schema: fullSchema,
			ETag:   "etag-2",
		},
	}
	client.dataset.tables["spans"] = table

	s := schemaStore(client)
	err = s.ensureSchema(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if table.updateCalled {
		t.Error("table.Update should NOT be called when schema is current")
	}
}

// ── evolveSchema Tests ───────────────────────────────────────────────

func TestEvolveSchema_AddsNewColumns(t *testing.T) {
	client := newMockBQClient()

	existingSchema := bq.Schema{
		{Name: "span_id", Type: bq.StringFieldType},
		{Name: "trace_id", Type: bq.StringFieldType},
	}
	desiredSchema := bq.Schema{
		{Name: "span_id", Type: bq.StringFieldType},
		{Name: "trace_id", Type: bq.StringFieldType},
		{Name: "name", Type: bq.StringFieldType, Required: true},
		{Name: "kind", Type: bq.StringFieldType, Required: true},
	}

	table := &mockBQTable{
		id: "spans",
		metadataResult: &bq.TableMetadata{
			Schema: existingSchema,
			ETag:   "etag-3",
		},
	}
	client.dataset.tables["spans"] = table

	s := schemaStore(client)
	err := s.evolveSchema(context.Background(), table, desiredSchema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !table.updateCalled {
		t.Fatal("expected table.Update to be called")
	}

	// Should have all 4 columns (2 existing + 2 new).
	if len(table.updateSchema) != 4 {
		t.Errorf("expected 4 columns in updated schema, got %d", len(table.updateSchema))
	}

	// New columns should be forced NULLABLE (Required=false).
	for _, field := range table.updateSchema {
		if field.Name == "name" || field.Name == "kind" {
			if field.Required {
				t.Errorf("new column %q should be NULLABLE (Required=false)", field.Name)
			}
		}
	}
}

func TestEvolveSchema_NoChanges(t *testing.T) {
	client := newMockBQClient()

	schema := bq.Schema{
		{Name: "span_id", Type: bq.StringFieldType},
		{Name: "trace_id", Type: bq.StringFieldType},
	}

	table := &mockBQTable{
		id: "spans",
		metadataResult: &bq.TableMetadata{
			Schema: schema,
			ETag:   "etag-4",
		},
	}
	client.dataset.tables["spans"] = table

	s := schemaStore(client)
	err := s.evolveSchema(context.Background(), table, schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if table.updateCalled {
		t.Error("table.Update should NOT be called when schema matches")
	}
}
