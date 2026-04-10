# ──────────────────────────────────────────────────
# Cloud Run — Candela server + IAP
# Single service: Go backend + proxy + embedded UI.
# ──────────────────────────────────────────────────

locals {
  image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.candela.repository_id}/candela-server:${var.image_tag}"
}

resource "google_cloud_run_v2_service" "candela" {
  provider = google-beta

  name     = var.service_name
  location = var.region

  # Set to true after initial deploy is confirmed working.
  deletion_protection = false

  # Enable IAP directly on the Cloud Run service (requires google-beta provider).
  iap_enabled = true

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

# ── Access Control ──
# IAP for Cloud Run requires:
# 1. iap.httpsResourceAccessor on the Cloud Run service's IAP resource
# 2. run.invoker for the IAP service agent (so IAP can forward requests)
# 3. run.invoker for the Google Group (belt + suspenders)

resource "google_iap_web_cloud_run_service_iam_member" "group_iap_access" {
  project                = var.project_id
  location               = var.region
  cloud_run_service_name = google_cloud_run_v2_service.candela.name
  role                   = "roles/iap.httpsResourceAccessor"
  member                 = "group:${var.iap_google_group}"
}

# IAP service agent needs run.invoker to forward authenticated requests.
resource "google_cloud_run_v2_service_iam_member" "iap_sa_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.candela.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:service-${data.google_project.current.number}@gcp-sa-iap.iam.gserviceaccount.com"
}

resource "google_cloud_run_v2_service_iam_member" "group_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.candela.name
  role     = "roles/run.invoker"
  member   = "group:${var.iap_google_group}"
}


# ── Data Sources ──

data "google_project" "current" {
  project_id = var.project_id
}
