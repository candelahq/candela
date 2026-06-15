# Deployment Examples

Reference configurations for self-hosting Candela.

## Firestore Security Rules

[`firestore.rules`](./firestore.rules) — defense-in-depth security rules for all Firestore collections.

These rules are **not deployed from this repo**. In production, they are managed via Terraform in the infrastructure repository using `google_firebaserules_ruleset` and `google_firebaserules_release` resources.

This file is provided as a reference for self-hosters who need to configure their own Firestore security rules.

### Collections covered

| Collection | Read | Write |
|---|---|---|
| `model_catalog` | Authenticated users | Admin only (custom claims) |
| `users/{userId}` | Owner or admin | Server-only |
| `users/{userId}/budgets` | Owner or admin | Server-only |
| `users/{userId}/grants` | Owner or admin | Server-only |
| `users/{userId}/audit` | Owner or admin | Server-only |
| `global_audit` | Admin only | Server-only |
| `rate_limit` | Denied | Denied |

> **Note:** All production access uses the Go Admin SDK, which bypasses security rules entirely. These rules exist as defense-in-depth against leaked API keys or accidental client SDK usage.
