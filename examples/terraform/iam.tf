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

# BigQuery: run queries (project-level)
resource "google_project_iam_member" "candela_bigquery_job" {
  project = var.project_id
  role    = "roles/bigquery.jobUser"
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

# Service Account Token Creator: scoped to self only (not project-wide)
# Needed for generating identity tokens for Vertex AI calls.
resource "google_service_account_iam_member" "candela_self_token_creator" {
  service_account_id = google_service_account.candela.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.candela.email}"
}

# ── IAP ID Token Creator (custom role) ──
# Narrow role that only allows generating OIDC ID tokens (getOpenIdToken).
# This lets candela-local users authenticate through IAP via SA impersonation
# WITHOUT being able to generate access tokens (which could call LLM APIs directly).

resource "google_project_iam_custom_role" "iap_id_token_creator" {
  role_id     = "iapIdTokenCreator"
  title       = "IAP ID Token Creator"
  description = "Allows generating OIDC ID tokens only (for IAP auth). Cannot generate access tokens."
  permissions = ["iam.serviceAccounts.getOpenIdToken"]
  stage       = "GA"
}

resource "google_service_account_iam_member" "candela_users_iap_token" {
  service_account_id = google_service_account.candela.name
  role               = google_project_iam_custom_role.iap_id_token_creator.id
  member             = "group:${var.invoker_google_group}"
}
