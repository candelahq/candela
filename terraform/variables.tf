# ──────────────────────────────────────────────────
# Candela — Terraform Variables
# ──────────────────────────────────────────────────

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for Cloud Run and Vertex AI"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone (used for some zonal resources)"
  type        = string
  default     = "us-central1-a"
}

# ── Cloud Run ──

variable "service_name" {
  description = "Cloud Run service name"
  type        = string
  default     = "candela"
}

variable "image_tag" {
  description = "Container image tag to deploy (e.g., 'latest', 'v1.0.0', commit SHA)"
  type        = string
  default     = "latest"
}

variable "min_instances" {
  description = "Minimum Cloud Run instances (0 = scale to zero)"
  type        = number
  default     = 0
}

variable "max_instances" {
  description = "Maximum Cloud Run instances"
  type        = number
  default     = 10
}

variable "cpu" {
  description = "CPU allocation per instance"
  type        = string
  default     = "1"
}

variable "memory" {
  description = "Memory allocation per instance"
  type        = string
  default     = "512Mi"
}

# ── Access Control ──

variable "invoker_google_group" {
  description = "Google Group email that gets Cloud Run invoker access"
  type        = string
}


# ── BigQuery ──

variable "bigquery_dataset" {
  description = "BigQuery dataset name for spans"
  type        = string
  default     = "candela"
}

variable "bigquery_location" {
  description = "BigQuery dataset location"
  type        = string
  default     = "US"
}

# ── Firestore ──

variable "firestore_location" {
  description = "Firestore database location"
  type        = string
  default     = "nam5"  # US multi-region
}

# ── Vertex AI ──

variable "vertex_ai_region" {
  description = "Vertex AI region for Claude proxy"
  type        = string
  default     = "us-east5"
}

# ── GitHub Actions CD ──

variable "github_repo" {
  description = "GitHub repository (owner/name) for Workload Identity Federation"
  type        = string
  default     = "candelahq/candela"
}
