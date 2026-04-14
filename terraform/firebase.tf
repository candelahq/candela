# ──────────────────────────────────────────────────
# Firebase — Auth, Web App, and App Hosting
# ──────────────────────────────────────────────────

# Enable Firebase on the existing GCP project.
resource "google_firebase_project" "default" {
  provider = google-beta
  project  = var.project_id

  depends_on = [google_project_service.apis]
}

# Firebase Web App (used by Next.js UI for auth config).
resource "google_firebase_web_app" "candela_ui" {
  provider     = google-beta
  project      = var.project_id
  display_name = "Candela UI"

  depends_on = [google_firebase_project.default]
}

# Retrieve the Firebase Web App config (apiKey, authDomain, etc.)
data "google_firebase_web_app_config" "candela_ui" {
  provider   = google-beta
  project    = var.project_id
  web_app_id = google_firebase_web_app.candela_ui.app_id
}

# ── Firebase Auth — Google Sign-In ──
# Note: Identity Platform (idp) config requires the google-beta provider.

resource "google_identity_platform_config" "auth" {
  provider = google-beta
  project  = var.project_id

  sign_in {
    allow_duplicate_emails = false

    email {
      enabled           = false
      password_required = false
    }
  }

  depends_on = [google_firebase_project.default]
}

resource "google_identity_platform_default_supported_idp_config" "google" {
  provider = google-beta
  project  = var.project_id
  idp_id   = "google.com"

  client_id     = var.google_oauth_client_id
  client_secret = var.google_oauth_client_secret

  enabled = true

  depends_on = [google_identity_platform_config.auth]
}
