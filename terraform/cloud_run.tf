# ──────────────────────────────────────────────────
# Cloud Run — Candela server (API + proxy)
# Protected by run.invoker IAM (no IAP).
# Programmatic access via ID tokens from candela-local.
# ──────────────────────────────────────────────────

locals {
  image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.candela.repository_id}/candela-server:${var.image_tag}"
}

resource "google_cloud_run_v2_service" "candela" {
  provider = google-beta

  name     = var.service_name
  location = var.region

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
      env {
        name  = "CANDELA_DEV_MODE"
        value = "false" # Firebase Auth validates tokens; set to true only for local dev
      }
      env {
        name  = "CLOUD_RUN_URL"
        value = "https://candela-6y6kmipuda-uc.a.run.app"
      }
    }
  }

  depends_on = [
    google_project_service.apis,
    google_artifact_registry_repository.candela,
  ]
}

# ── Access Control ──
# Cloud Run is NOT publicly accessible. Access requires:
# 1. A valid Google ID token (audience = Cloud Run service URL)
# 2. The caller must have roles/run.invoker on the service
# candela-local injects ID tokens automatically for developer tools.

resource "google_cloud_run_v2_service_iam_member" "group_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.candela.name
  role     = "roles/run.invoker"
  member   = "group:${var.invoker_google_group}"
}


# ── Data Sources ──

data "google_project" "current" {
  project_id = var.project_id
}
