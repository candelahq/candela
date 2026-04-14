# ──────────────────────────────────────────────────
# Outputs — Values needed for deployment and candela-local config
# ──────────────────────────────────────────────────

output "cloud_run_url" {
  description = "Cloud Run service URL"
  value       = google_cloud_run_v2_service.candela.uri
}

output "artifact_registry_repo" {
  description = "Docker image repository for pushing Candela images"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.candela.repository_id}"
}

output "bigquery_table" {
  description = "BigQuery spans table fully-qualified name"
  value       = "${var.project_id}.${google_bigquery_dataset.candela.dataset_id}.${google_bigquery_table.spans.table_id}"
}

output "service_account_email" {
  description = "Candela service account email"
  value       = google_service_account.candela.email
}

output "candela_local_config" {
  description = "Template for ~/.candela.yaml"
  value       = <<-EOT
    # ~/.candela.yaml
    remote: ${google_cloud_run_v2_service.candela.uri}
    audience: ${google_cloud_run_v2_service.candela.uri}
    port: 8181
  EOT
}

output "firebase_config" {
  description = "Firebase Web App config for the Next.js UI"
  value = {
    api_key     = data.google_firebase_web_app_config.candela_ui.api_key
    auth_domain = "${var.project_id}.firebaseapp.com"
    project_id  = var.project_id
    app_id      = google_firebase_web_app.candela_ui.app_id
  }
}
