# ──────────────────────────────────────────────────
# Load Balancer + IAP — Custom domain with zero-trust access
# ──────────────────────────────────────────────────
#
# Provisions a Global HTTPS Load Balancer in front of Cloud Run
# with Identity-Aware Proxy (IAP) for access control.
#
# Traffic flow:
#   Browser/candela-local → GLB (IAP enforced) → Cloud Run (internal only)
#
# Only members of var.invoker_google_group can access.

locals {
  has_custom_domain = var.custom_domain != ""
}

# ── Static IP ──

resource "google_compute_global_address" "candela" {
  count = local.has_custom_domain ? 1 : 0
  name  = "${var.service_name}-lb-ip"
}

# ── SSL Certificate ──

resource "google_compute_managed_ssl_certificate" "candela" {
  count = local.has_custom_domain ? 1 : 0
  name  = "${var.service_name}-ssl-cert"

  managed {
    domains = [var.custom_domain]
  }
}

# ── Serverless NEG (Cloud Run backend) ──

resource "google_compute_region_network_endpoint_group" "candela" {
  count                 = local.has_custom_domain ? 1 : 0
  name                  = "${var.service_name}-serverless-neg"
  region                = var.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = google_cloud_run_v2_service.candela.name
  }
}

# ── Backend Service (with IAP) ──

resource "google_compute_backend_service" "candela" {
  count                 = local.has_custom_domain ? 1 : 0
  name                  = "${var.service_name}-backend"
  protocol              = "HTTP"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  timeout_sec           = 30

  backend {
    group           = google_compute_region_network_endpoint_group.candela[0].id
  }

  # Enable IAP when OAuth credentials are provided.
  # Phase 1: Apply without IAP (set up infra).
  # Phase 2: Create OAuth client, set vars, apply again to enable IAP.
  dynamic "iap" {
    for_each = var.iap_oauth_client_id != "" ? [1] : []
    content {
      enabled              = true
      oauth2_client_id     = var.iap_oauth_client_id
      oauth2_client_secret = var.iap_oauth_client_secret
    }
  }

  log_config {
    enable      = true
    sample_rate = 0.1
  }
}

# ── URL Maps ──

resource "google_compute_url_map" "candela" {
  count           = local.has_custom_domain ? 1 : 0
  name            = "${var.service_name}-url-map"
  default_service = google_compute_backend_service.candela[0].id
}

# HTTP → HTTPS redirect.
resource "google_compute_url_map" "candela_redirect" {
  count = local.has_custom_domain ? 1 : 0
  name  = "${var.service_name}-http-redirect"

  default_url_redirect {
    https_redirect         = true
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
    strip_query            = false
  }
}

# ── Proxies ──

resource "google_compute_target_https_proxy" "candela" {
  count            = local.has_custom_domain ? 1 : 0
  name             = "${var.service_name}-https-proxy"
  url_map          = google_compute_url_map.candela[0].id
  ssl_certificates = [google_compute_managed_ssl_certificate.candela[0].id]
}

resource "google_compute_target_http_proxy" "candela_redirect" {
  count   = local.has_custom_domain ? 1 : 0
  name    = "${var.service_name}-http-proxy"
  url_map = google_compute_url_map.candela_redirect[0].id
}

# ── Forwarding Rules ──

resource "google_compute_global_forwarding_rule" "candela_https" {
  count                 = local.has_custom_domain ? 1 : 0
  name                  = "${var.service_name}-https-rule"
  ip_address            = google_compute_global_address.candela[0].address
  ip_protocol           = "TCP"
  port_range            = "443"
  target                = google_compute_target_https_proxy.candela[0].id
  load_balancing_scheme = "EXTERNAL_MANAGED"
}

resource "google_compute_global_forwarding_rule" "candela_http" {
  count                 = local.has_custom_domain ? 1 : 0
  name                  = "${var.service_name}-http-rule"
  ip_address            = google_compute_global_address.candela[0].address
  ip_protocol           = "TCP"
  port_range            = "80"
  target                = google_compute_target_http_proxy.candela_redirect[0].id
  load_balancing_scheme = "EXTERNAL_MANAGED"
}

# ── IAP (Identity-Aware Proxy) ──
# Zero-trust access: only members of the invoker Google Group can reach
# Candela through the load balancer. No allUsers needed.
#
# Prerequisites (one-time manual setup):
#   1. Go to GCP Console → APIs & Services → OAuth consent screen
#   2. Configure the consent screen (Internal)
#   3. Go to Credentials → Create OAuth client ID → Web application
#   4. Set the client ID and secret in terraform.tfvars:
#        iap_oauth_client_id     = "xxx.apps.googleusercontent.com"
#        iap_oauth_client_secret = "GOCSPX-xxx"

# Grant the Google Group access through IAP.
resource "google_iap_web_backend_service_iam_member" "candela_users" {
  count               = (local.has_custom_domain && var.iap_oauth_client_id != "") ? 1 : 0
  web_backend_service = google_compute_backend_service.candela[0].name
  role                = "roles/iap.httpsResourceAccessor"
  member              = "group:${var.invoker_google_group}"
}

# ── Outputs ──

output "load_balancer_ip" {
  description = "Static IP for DNS A record"
  value       = local.has_custom_domain ? google_compute_global_address.candela[0].address : null
}

output "custom_domain_url" {
  description = "Custom domain URL"
  value       = local.has_custom_domain ? "https://${var.custom_domain}" : null
}
