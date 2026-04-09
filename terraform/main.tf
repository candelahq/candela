# ──────────────────────────────────────────────────
# Candela — Terraform Main
# Provider config, backend, and API enablement.
# ──────────────────────────────────────────────────

terraform {
  required_version = ">= 1.5"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }

  # Remote state in GCS — create this bucket manually first:
  #   gsutil mb -p $PROJECT_ID -l $REGION gs://$PROJECT_ID-terraform-state
  backend "gcs" {
    # Configured via -backend-config or terraform.tfvars
    # bucket = "<project-id>-terraform-state"
    prefix = "candela"
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# ── Enable required GCP APIs ──

resource "google_project_service" "apis" {
  for_each = toset([
    "run.googleapis.com",
    "artifactregistry.googleapis.com",
    "bigquery.googleapis.com",
    "firestore.googleapis.com",
    "iap.googleapis.com",
    "iam.googleapis.com",
    "aiplatform.googleapis.com",
    "cloudresourcemanager.googleapis.com",
  ])

  service            = each.value
  disable_on_destroy = false
}
