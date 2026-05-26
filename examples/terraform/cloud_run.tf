# ──────────────────────────────────────────────────
# Cloud Run — Candela server (API + proxy)
# Protected by run.invoker IAM, or by IAP when a
# custom domain + OAuth credentials are provided.
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

  ingress = (var.custom_domain != "" && var.iap_oauth_client_id != "") ? "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER" : "INGRESS_TRAFFIC_ALL"

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
      env {
        name  = "CANDELA_DEV_MODE"
        value = "false" # Firebase Auth validates tokens; set to true only for local dev
      }
      env {
        name  = "CLOUD_RUN_URL"
        value = var.cloud_run_url
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

# When IAP is enabled, Cloud Run needs allUsers invoker because ingress is
# restricted to the Internal Load Balancer. IAP handles the access gate.
resource "google_cloud_run_v2_service_iam_member" "allow_unauthenticated" {
  count    = (var.custom_domain != "" && var.iap_oauth_client_id != "") ? 1 : 0
  project  = google_cloud_run_v2_service.candela.project
  location = google_cloud_run_v2_service.candela.location
  name     = google_cloud_run_v2_service.candela.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

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
