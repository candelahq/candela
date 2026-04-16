# ──────────────────────────────────────────────────
# Firebase — Project, Web App, and Auth baseline
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

# ── Firebase Auth ──
# Terraform configures the Identity Platform base settings.
# Google Sign-In is enabled via Firebase Console (one-time toggle):
#   Firebase Console → Authentication → Sign-in method → Google → Enable
#
# This avoids needing custom OAuth client credentials in Terraform.

resource "google_identity_platform_config" "auth" {
  provider = google-beta
  project  = var.project_id

  authorized_domains = [
    "localhost",
    "${var.project_id}.firebaseapp.com",
    "${var.project_id}.web.app",
    "candela-6y6kmipuda-uc.a.run.app",
    "candela-${data.google_project.current.number}.${var.region}.run.app",
  ]

  sign_in {
    allow_duplicate_emails = false

    email {
      enabled           = false
      password_required = false
    }
  }

  depends_on = [google_firebase_project.default]
}
