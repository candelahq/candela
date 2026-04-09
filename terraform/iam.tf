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

# BigQuery: read + write spans
resource "google_project_iam_member" "candela_bigquery" {
  project = var.project_id
  role    = "roles/bigquery.dataEditor"
  member  = "serviceAccount:${google_service_account.candela.email}"
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

# Service Account Token Creator: generate identity tokens for Vertex AI
resource "google_project_iam_member" "candela_token_creator" {
  project = var.project_id
  role    = "roles/iam.serviceAccountTokenCreator"
  member  = "serviceAccount:${google_service_account.candela.email}"
}
