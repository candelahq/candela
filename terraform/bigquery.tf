# ──────────────────────────────────────────────────
# BigQuery — Spans storage (OLAP)
# Schema mirrors proto/candela/types/trace.proto
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

  # Schema matches the Go storage.Span struct and proto definitions.
  schema = jsonencode([
    { name = "span_id", type = "STRING", mode = "REQUIRED" },
    { name = "trace_id", type = "STRING", mode = "REQUIRED" },
    { name = "parent_span_id", type = "STRING", mode = "NULLABLE" },
    { name = "name", type = "STRING", mode = "REQUIRED" },
    { name = "kind", type = "INTEGER", mode = "REQUIRED" },
    { name = "status", type = "INTEGER", mode = "REQUIRED" },
    { name = "status_message", type = "STRING", mode = "NULLABLE" },
    { name = "start_time", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "end_time", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "duration_ns", type = "INT64", mode = "REQUIRED" },
    { name = "project_id", type = "STRING", mode = "NULLABLE" },
    { name = "environment", type = "STRING", mode = "NULLABLE" },
    { name = "service_name", type = "STRING", mode = "NULLABLE" },
    { name = "user_id", type = "STRING", mode = "NULLABLE" },
    # GenAI attributes
    { name = "gen_ai_model", type = "STRING", mode = "NULLABLE" },
    { name = "gen_ai_provider", type = "STRING", mode = "NULLABLE" },
    { name = "gen_ai_input_tokens", type = "INT64", mode = "NULLABLE" },
    { name = "gen_ai_output_tokens", type = "INT64", mode = "NULLABLE" },
    { name = "gen_ai_total_tokens", type = "INT64", mode = "NULLABLE" },
    { name = "gen_ai_cost_usd", type = "FLOAT64", mode = "NULLABLE" },
    # Structured attributes
    {
      name = "attributes",
      type = "RECORD",
      mode = "REPEATED",
      fields = [
        { name = "key", type = "STRING", mode = "REQUIRED" },
        { name = "value", type = "STRING", mode = "REQUIRED" },
      ]
    },
  ])

  deletion_protection = true
}
