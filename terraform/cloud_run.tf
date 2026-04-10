# ──────────────────────────────────────────────────
# Cloud Run — Candela server + IAP
# Single service: Go backend + proxy + embedded UI.
# ──────────────────────────────────────────────────

locals {
  image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.candela.repository_id}/candela-server:${var.image_tag}"
}

resource "google_cloud_run_v2_service" "candela" {
  name     = var.service_name
  location = var.region

  # Prevent accidental deletion in production.
  deletion_protection = true

  template {
    service_account = google_service_account.candela.email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = local.image

      ports {
        container_port = 3000
      }

      resources {
        limits = {
          cpu    = var.cpu
          memory = var.memory
        }
      }

      # ── Environment variables ──
      env {
        name  = "CANDELA_STORAGE_BACKEND"
        value = "bigquery"
      }
      env {
        name  = "CANDELA_BQ_PROJECT"
        value = var.project_id
      }
      env {
        name  = "CANDELA_BQ_DATASET"
        value = var.bigquery_dataset
      }
      env {
        name  = "CANDELA_BQ_LOCATION"
        value = var.bigquery_location
      }
      env {
        name  = "CANDELA_FIRESTORE_DATABASE"
        value = google_firestore_database.candela.name
      }
      env {
        name  = "CANDELA_FIRESTORE_PROJECT"
        value = var.project_id
      }
      env {
        name  = "CANDELA_VERTEX_PROJECT"
        value = var.project_id
      }
      env {
        name  = "CANDELA_VERTEX_REGION"
        value = var.vertex_ai_region
      }
      env {
        name  = "CANDELA_PROXY_ENABLED"
        value = "true"
      }
    }
  }

  depends_on = [
    google_project_service.apis,
    google_artifact_registry_repository.candela,
  ]
}

# ── IAP Configuration ──
# Require authentication — IAP handles it.
# No unauthenticated access to the Cloud Run service.

resource "google_cloud_run_v2_service_iam_member" "iap_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.candela.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-iap.iam.gserviceaccount.com"
}

# Grant the Google Group access through IAP.
resource "google_iap_web_iam_member" "group_access" {
  project = var.project_id
  role    = "roles/iap.httpsResourceAccessUser"
  member  = "group:${var.iap_google_group}"
}

# ── IAP Enablement ──
# IAP is enabled via gcloud (not Terraform — the Terraform resources are deprecated).
# Run once after initial deploy:
#
#   gcloud run services update candela --region=REGION --iap
#   gcloud run services describe candela --region=REGION \
#     --format='value(metadata.annotations."run.googleapis.com/iap-client-id")'
#
# The client ID from above is the `audience` value for candela-local (~/.candela.yaml).

# ── Data Sources ──

data "google_project" "current" {
  project_id = var.project_id
}
