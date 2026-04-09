# ──────────────────────────────────────────────────
# IAM — Service account and role bindings
# ──────────────────────────────────────────────────

# Service account for the Candela Cloud Run service.
resource "google_service_account" "candela" {
  account_id   = "candela-server"
  display_name = "Candela Server"
  description  = "Service identity for the Candela Cloud Run service"
}

# ── Role bindings for the service account ──

# BigQuery: read + write spans (scoped to candela dataset only)
resource "google_bigquery_dataset_iam_member" "candela_bigquery" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.candela.dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${google_service_account.candela.email}"
}

# Firestore: read + write users, budgets, grants
resource "google_project_iam_member" "candela_firestore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.candela.email}"
}

# Vertex AI: proxy LLM requests to Claude
resource "google_project_iam_member" "candela_vertex_ai" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = "serviceAccount:${google_service_account.candela.email}"
}

# Service Account Token Creator: scoped to self only (not project-wide)
# Needed for generating identity tokens for Vertex AI calls.
resource "google_service_account_iam_member" "candela_self_token_creator" {
  service_account_id = google_service_account.candela.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.candela.email}"
}
