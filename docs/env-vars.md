# 🌐 Environment Variable Reference

Candela uses environment variables for production configuration, especially in containerized deployments where config files are generated at runtime.

## Server Environment Variables

These are used by the container entrypoint (`deploy/entrypoint.sh`) to generate `config.yaml` at startup.

### Storage

| Variable | Default | Description |
|----------|---------|-------------|
| `CANDELA_STORAGE_BACKEND` | `duckdb` | Storage backend: `duckdb`, `sqlite`, or `bigquery` |
| `CANDELA_BQ_PROJECT` | _(empty)_ | BigQuery GCP project ID |
| `CANDELA_BQ_DATASET` | `candela` | BigQuery dataset name |
| `CANDELA_BQ_LOCATION` | `US` | BigQuery dataset location |

### Proxy

| Variable | Default | Description |
|----------|---------|-------------|
| `CANDELA_PROXY_ENABLED` | `false` | Enable the LLM API proxy |
| `CANDELA_VERTEX_PROJECT` | _(empty)_ | GCP project for Vertex AI (required for Anthropic proxy) |
| `CANDELA_VERTEX_REGION` | `us-east5` | Vertex AI region (must have Claude access) |
| `CANDELA_LMSTUDIO_ENABLED` | `true` | Enable LM Studio compatible endpoints |
| `CANDELA_LMSTUDIO_PORT` | `1234` | Secondary listener port for LM Studio compat |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `CANDELA_DEV_MODE` | `false` | Skip auth, inject synthetic admin user. **Never use in production.** |
| `CLOUD_RUN_URL` | _(empty)_ | Cloud Run service URL. Used as the audience for Google ID token validation (Strategy 2). Set this to enable `candela-local` Team Mode auth. |

### Firestore

| Variable | Default | Description |
|----------|---------|-------------|
| `CANDELA_FIRESTORE_PROJECT` | _(empty)_ | GCP project for Firestore |
| `CANDELA_FIRESTORE_DATABASE` | `candela` | Firestore database ID (use `(default)` for the default database) |

### Config

| Variable | Default | Description |
|----------|---------|-------------|
| `CANDELA_CONFIG` | `config.yaml` | Path to the config file. In containers, set to `/etc/candela/config.yaml` by the entrypoint. |

---

## Build-Time Variables (Docker)

These are passed as `--build-arg` during `docker build` and baked into the Next.js client bundle. They are **public** (non-secret) values.

| Variable | Description |
|----------|-------------|
| `NEXT_PUBLIC_FIREBASE_API_KEY` | Firebase Web API key |
| `NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN` | Firebase Auth domain (e.g., `your-project.firebaseapp.com`) |
| `NEXT_PUBLIC_FIREBASE_PROJECT_ID` | Firebase project ID |
| `NEXT_PUBLIC_FIREBASE_APP_ID` | Firebase app ID |

These are configured in `cloudbuild.yaml` as substitution variables:

```bash
gcloud builds submit --project $PROJECT \
  --substitutions=\
_FIREBASE_API_KEY=AIza...,\
_FIREBASE_AUTH_DOMAIN=my-project.firebaseapp.com,\
_FIREBASE_PROJECT_ID=my-project,\
_FIREBASE_APP_ID=1:123:web:abc
```

---

## UI Environment Variables

The Next.js UI reads these at **build time** (prefixed with `NEXT_PUBLIC_`) and at **runtime**:

### Build-Time (`NEXT_PUBLIC_*`)

| Variable | Default | Description |
|----------|---------|-------------|
| `NEXT_PUBLIC_API_URL` | `""` (empty) | Backend API URL. Empty = relative URLs (same-origin, proxied via Next.js rewrites). Set to `http://localhost:8181` for local dev with separate backend. |
| `NEXT_PUBLIC_FIREBASE_API_KEY` | _(required)_ | Firebase Web API key |
| `NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN` | _(required)_ | Firebase Auth domain |
| `NEXT_PUBLIC_FIREBASE_PROJECT_ID` | _(required)_ | Firebase project ID |
| `NEXT_PUBLIC_FIREBASE_APP_ID` | _(required)_ | Firebase app ID |

### Runtime

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_URL` | `http://localhost:8181` | Go backend URL for Next.js API rewrites (set in `next.config.ts`) |
| `PORT` | `3000` | Next.js server port |
| `HOSTNAME` | `0.0.0.0` | Next.js server bind address |

---

## candela-local Environment

`candela-local` primarily reads from `~/.candela.yaml` (see [docs/candela-local.md](candela-local.md)). It also respects:

| Variable | Description |
|----------|-------------|
| `GOOGLE_APPLICATION_CREDENTIALS` | Path to GCP service account key (alternative to `gcloud auth application-default login`) |
| `GOOGLE_CLOUD_PROJECT` | Fallback GCP project if not set in `~/.candela.yaml` |

---

## CI Environment Variables

| Variable | Used In | Description |
|----------|---------|-------------|
| `BUF_TOKEN` | CI workflow | Buf registry token for remote proto generation |
| `GITHUB_TOKEN` | CI workflow | GitHub token for Nix installer |
| `CI` | Playwright | Set to `true` to run browsers in CI mode |

---

## Quick Reference: Production Deployment

Minimal environment for a production Cloud Run deployment:

```bash
# Required
CANDELA_STORAGE_BACKEND=bigquery
CANDELA_BQ_PROJECT=my-gcp-project
CANDELA_FIRESTORE_PROJECT=my-gcp-project
CANDELA_PROXY_ENABLED=true
CANDELA_VERTEX_PROJECT=my-gcp-project

# Recommended
CANDELA_VERTEX_REGION=us-east5
CANDELA_BQ_DATASET=candela
CANDELA_FIRESTORE_DATABASE=candela
CLOUD_RUN_URL=https://candela-xxx.a.run.app

# Optional
CANDELA_DEV_MODE=false  # default
CANDELA_LMSTUDIO_ENABLED=true  # default
```
