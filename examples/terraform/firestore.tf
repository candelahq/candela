# ──────────────────────────────────────────────────
# Firestore — Transactional data
# Users, budgets, grants, projects, audit, rate limits.
# ──────────────────────────────────────────────────

resource "google_firestore_database" "candela" {
  name        = "candela"
  location_id = var.firestore_location
  type        = "FIRESTORE_NATIVE"

  # Prevent accidental data loss on terraform destroy.
  deletion_policy = "ABANDON"

  depends_on = [google_project_service.apis]
}

# ── Composite indexes for common query patterns ──

# Budget lookup: current period for a user
resource "google_firestore_index" "user_budgets_by_period" {
  database   = google_firestore_database.candela.name
  collection = "budgets"

  fields {
    field_path = "userId"
    order      = "ASCENDING"
  }
  fields {
    field_path = "periodKey"
    order      = "DESCENDING"
  }
}

# Active grants: not expired, ordered by expiry
resource "google_firestore_index" "user_grants_active" {
  database   = google_firestore_database.candela.name
  collection = "grants"

  fields {
    field_path = "userId"
    order      = "ASCENDING"
  }
  fields {
    field_path = "expiresAt"
    order      = "ASCENDING"
  }
}

# Audit log: most recent first
resource "google_firestore_index" "user_audit_log" {
  database   = google_firestore_database.candela.name
  collection = "audit"

  fields {
    field_path = "userId"
    order      = "ASCENDING"
  }
  fields {
    field_path = "timestamp"
    order      = "DESCENDING"
  }
}

# ── TTL policy for rate limit documents (auto-cleanup) ──

resource "google_firestore_field" "rate_limit_ttl" {
  database   = google_firestore_database.candela.name
  collection = "rate_limit"
  field      = "expireAt"

  ttl_config {}
}
