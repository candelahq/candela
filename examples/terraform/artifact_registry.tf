# ──────────────────────────────────────────────────
# Artifact Registry — Container image repository
# ──────────────────────────────────────────────────

resource "google_artifact_registry_repository" "candela" {
  location      = var.region
  repository_id = "candela"
  description   = "Candela container images"
  format        = "DOCKER"

  cleanup_policies {
    id     = "keep-recent"
    action = "KEEP"
    most_recent_versions {
      keep_count = 10
    }
  }

  depends_on = [google_project_service.apis]
}
