# Generic OIDC Authentication

Candela supports any OIDC-compliant identity provider through config-driven
resolver chains. This replaces the need for Firebase Auth as the sole identity
backend, enabling enterprises using Zitadel, Auth0, Okta, Keycloak, or Cognito
to adopt Candela without changing their existing identity infrastructure.

## Quick Start

Add your OIDC provider to `config.yaml`:

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://your-idp.example.com
      audience: candela-server
```

That's it. Candela auto-discovers the JWKS endpoint from your issuer's
`.well-known/openid-configuration` and validates tokens at request time.

## How It Works

1. Client sends `Authorization: Bearer <id_token>` with a token from your OIDC provider
2. Candela's resolver chain tries each configured resolver in order
3. The OIDC resolver verifies the token signature via JWKS, checks audience and expiry
4. On success, the `sub` and `email` claims become the user's identity
5. Normal budget enforcement, RBAC, and tenant validation apply

## Provider-Specific Configs

### Zitadel

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://your-instance.zitadel.cloud
      audience: your-project-id
      claim_mapping:
        tenants: "urn:zitadel:iam:org:project:roles"
```

### Auth0

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://your-tenant.auth0.com
      audience: https://candela-api
```

### Okta

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://your-org.okta.com/oauth2/default
      audience: candela
```

### Keycloak

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://keycloak.example.com/realms/your-realm
      audience: candela-server
```

### AWS Cognito

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://cognito-idp.us-east-1.amazonaws.com/us-east-1_XXXXXXXXX
      audience: your-cognito-client-id
```

## Migration from Firebase

Add the OIDC resolver **before** Firebase to prioritize new tokens while
maintaining backward compatibility for existing Firebase sessions:

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://your-idp.example.com
      audience: candela-server
    - type: firebase        # fallback for existing browser sessions
    - type: google_oauth    # fallback for CLI users (candela auth login)
```

The resolver chain tries each resolver in order. The first resolver that
recognizes the token wins. Unrecognized tokens fall through to the next resolver.

## Claim Mapping

By default, Candela maps these standard OIDC claims:

| Claim | Default | Maps to |
|:---|:---|:---|
| `sub` | Subject | `Identity.ID` |
| `email` | Email | `Identity.Email` |
| `roles` | Roles | (future: RBAC) |
| `candela.tenants` | Tenant IDs | `Identity.TenantIDs` |

Override any claim name with `claim_mapping`:

```yaml
auth:
  resolvers:
    - type: oidc
      issuer: https://your-idp.example.com
      audience: candela-server
      claim_mapping:
        subject: "sub"
        email: "email"
        roles: "custom:roles"
        tenants: "custom:tenant_ids"
```

Most providers use standard claims — you only need `claim_mapping` if your
provider uses custom claim names for tenant or role information.

## Resolver Types

| Type | Description | When to Use |
|:---|:---|:---|
| `oidc` | Generic OIDC with auto-discovery | Any OIDC provider |
| `firebase` | Firebase Auth ID tokens | Browser UI with Firebase Auth SDK |
| `google_oidc` | Google ID tokens (Cloud Run) | `candela-local` behind Cloud Run |
| `google_oauth` | Google OAuth2 access tokens | `candela auth login --provider gcp` |

## Tenant Validation (Planned)

> **Note**: The `TenantValidator` infrastructure exists internally but is not yet
> exposed as a config option. When wired, you'll be able to switch to `verified`
> tenant mode so that `X-Candela-Tenant-Id` headers are validated against signed
> JWT claims.

For now, configure `claim_mapping.tenants` to extract tenant IDs from your OIDC
tokens. The tenant IDs are populated on `Identity.TenantIDs` and available for
application-level validation.

## Backward Compatibility

If `auth.resolvers` is not set in your config, Candela uses the legacy
Firebase-based chain (Firebase → Google OIDC → Google OAuth). **Existing
deployments require zero config changes.**
