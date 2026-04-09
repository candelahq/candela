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

  # Delete protection for production.
  deletion_protection = false

  template {
    service_account = google_service_account.candela.email

    scaling {
      min_instance_count = var.min_instances
      max_instance_count = var.max_instances
    }

    containers {
      image = local.image

      ports {
        container_port = 8181
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

# NOTE: The google_iap_brand Terraform resource is deprecated (July 2025+).
# Create the OAuth consent screen and client manually in the Google Cloud Console:
#   1. Go to APIs & Services → OAuth consent screen → configure
#   2. Go to APIs & Services → Credentials → Create OAuth 2.0 Client ID
#   3. Set the client ID in terraform.tfvars as iap_oauth_client_id
# Variable defined in variables.tf

# ── Data Sources ──

data "google_project" "current" {
  project_id = var.project_id
}
