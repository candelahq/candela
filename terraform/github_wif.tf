# ──────────────────────────────────────────────────
# GitHub Actions — Workload Identity Federation
# ──────────────────────────────────────────────────
#
# Keyless authentication from GitHub Actions to GCP.
# No service account keys — GitHub OIDC tokens are exchanged
# for short-lived GCP credentials.
#
# After applying, set these GitHub secrets:
#   GCP_WIF_PROVIDER         = google_iam_workload_identity_pool_provider.github.name
#   GCP_WIF_SERVICE_ACCOUNT  = google_service_account.github_deploy.email

# Service account used by GitHub Actions for build + deploy.
resource "google_service_account" "github_deploy" {
  account_id   = "github-deploy"
  display_name = "GitHub Actions Deploy"
  description  = "Used by GitHub Actions CD workflow for Cloud Build + Cloud Run deploys"
}

# ── Workload Identity Pool ──

resource "google_iam_workload_identity_pool" "github" {
  workload_identity_pool_id = "github-actions"
  display_name              = "GitHub Actions"
  description               = "OIDC identity pool for GitHub Actions"
}

resource "google_iam_workload_identity_pool_provider" "github" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-oidc"
  display_name                       = "GitHub OIDC"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.actor"      = "assertion.actor"
    "attribute.repository" = "assertion.repository"
  }

  # Only allow tokens from the candelahq/candela repo.
  attribute_condition = "assertion.repository == '${var.github_repo}'"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# ── Role bindings for the deploy service account ──

# Allow GitHub Actions to impersonate the deploy SA.
resource "google_service_account_iam_member" "github_wif" {
  service_account_id = google_service_account.github_deploy.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repo}"
}

# Cloud Build: submit builds.
resource "google_project_iam_member" "github_cloudbuild" {
  project = var.project_id
  role    = "roles/cloudbuild.builds.editor"
  member  = "serviceAccount:${google_service_account.github_deploy.email}"
}

# Artifact Registry: push images.
resource "google_project_iam_member" "github_artifact_registry" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${google_service_account.github_deploy.email}"
}

# Cloud Run: deploy new revisions.
resource "google_project_iam_member" "github_cloud_run" {
  project = var.project_id
  role    = "roles/run.developer"
  member  = "serviceAccount:${google_service_account.github_deploy.email}"
}

# Act as the Cloud Run service account (for deploy).
resource "google_service_account_iam_member" "github_act_as_candela" {
  service_account_id = google_service_account.candela.name
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_deploy.email}"
}

# Cloud Build service account needs to read/write to storage for build logs.
resource "google_project_iam_member" "github_storage" {
  project = var.project_id
  role    = "roles/storage.admin"
  member  = "serviceAccount:${google_service_account.github_deploy.email}"
}

# Cloud Build requires the caller to have serviceAccountUser on the build SA.
# Uses data.google_project.current from cloud_run.tf for project number.

resource "google_service_account_iam_member" "github_act_as_cloudbuild" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/${data.google_project.current.number}@cloudbuild.gserviceaccount.com"
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_deploy.email}"
}

# Also act as the Compute Engine default SA — Cloud Build uses this as
# its execution identity when no custom SA is specified.
resource "google_service_account_iam_member" "github_act_as_compute" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/${data.google_project.current.number}-compute@developer.gserviceaccount.com"
  role               = "roles/iam.serviceAccountUser"
  member             = "serviceAccount:${google_service_account.github_deploy.email}"
}
