# ──────────────────────────────────────────────────
# BigQuery — Spans storage (OLAP)
# Schema generated from proto/candela/types/bq_span.proto
# Regenerate: cd proto && buf generate
# ──────────────────────────────────────────────────

resource "google_bigquery_dataset" "candela" {
  dataset_id    = var.bigquery_dataset
  friendly_name = "Candela"
  description   = "LLM observability spans and traces"
  location      = var.bigquery_location

  labels = {
    app = "candela"
  }

  depends_on = [google_project_service.apis]
}

resource "google_bigquery_table" "spans" {
  dataset_id = google_bigquery_dataset.candela.dataset_id
  table_id   = "spans"

  # Time partitioning on start_time for efficient time-range queries.
  time_partitioning {
    type  = "DAY"
    field = "start_time"
  }

  # Clustering for common access patterns.
  clustering = ["user_id", "project_id", "trace_id"]

  # Schema is generated from proto/candela/types/bq_span.proto
  # via protoc-gen-bq-schema. Do NOT hand-edit — update the proto instead.
  schema = file("${path.module}/../gen/bq/candela/types/spans.schema")

  deletion_protection = true
}
