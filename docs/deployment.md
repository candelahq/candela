# Candela Deployment Architecture

## Production Architecture

```mermaid
graph TB
    subgraph "Developer Machine"
        OC[OpenCode / Zed / Cursor]
        CL["candela-local<br/>(auth proxy)"]
        OC -->|"localhost:8181"| CL
    end

    subgraph "Google Cloud"
        subgraph "Cloud Run Container"
            UI["Next.js UI :3000"]
            GO["Go Backend :8181"]
            UI -->|"proxy rewrite"| GO
        end
        FA["Firebase Auth"]
        BQ[(BigQuery)]
        FS[(Firestore)]
        VAI[Vertex AI / OpenAI]
    end

    Browser -->|"HTTPS"| UI
    Browser <-->|"Google Sign-In"| FA
    CL -->|"Bearer token"| GO
    GO --> BQ
    GO --> FS
    GO -->|"proxy"| VAI
```

## Components

| Component | Location | Purpose |
|---|---|---|
| **Go Backend** | `cmd/candela-server` | API, LLM proxy, span ingestion, auth middleware, storage |
| **Next.js UI** | `ui/` | Dashboard, trace waterfall, costs, admin panel |
| **candela-local** | `cmd/candela-local` | CLI proxy injecting Google credentials for dev tools |
| **Terraform** | `terraform/` | Cloud Run, BigQuery, Firestore, Firebase, IAM |

## Authentication (3 Strategies)

```mermaid
flowchart TD
    REQ[Request] --> TOK{Has Bearer Token?}
    TOK -->|no| DENY[401]
    TOK -->|yes| S1[Strategy 1: Firebase ID Token]
    S1 -->|valid| OK[Authenticated]
    S1 -->|invalid| S2[Strategy 2: Google ID Token]
    S2 -->|valid| OK
    S2 -->|invalid| S3[Strategy 3: OAuth2 Access Token via userinfo]
    S3 -->|valid| OK
    S3 -->|invalid| DENY
```

| Strategy | Used By | Token Source |
|---|---|---|
| Firebase ID Token | Browser UI | Firebase JS SDK |
| Google ID Token | Service accounts | `idtoken.NewTokenSource()` |
| OAuth2 Access Token | candela-local (user ADC) | `gcloud auth application-default login` |

## Container Layout

```
entrypoint.sh starts:
  1. Go backend (port 8181, background)
  2. Next.js standalone (port 3000, foreground)

Next.js rewrites:
  /proxy/*         → localhost:8181
  /candela.v1.*    → localhost:8181
  /healthz         → localhost:8181
```

## candela-local Setup

```yaml
# ~/.candela.yaml
remote: https://candela-xxx.run.app
audience: https://candela-xxx.run.app
port: 8181
```

```bash
go run ./cmd/candela-local   # or: go install && candela-local
```

Point tools at `http://localhost:8181/proxy/openai/v1`.

## Terraform Resources

| File | Resources |
|---|---|
| `cloud_run.tf` | Cloud Run service, IAM |
| `firebase.tf` | Firebase project, Identity Platform, authorized domains |
| `bigquery.tf` | Dataset + spans table (time-partitioned) |
| `firestore.tf` | Firestore database |
| `iam.tf` | Service account + role bindings |
| `artifact_registry.tf` | Container image registry |

## Build & Deploy

```bash
# Build
gcloud builds submit --project $PROJECT -f deploy/cloudbuild.yaml .

# Deploy
gcloud run services update candela --project $PROJECT --region $REGION \
  --image $REGION-docker.pkg.dev/$PROJECT/candela/candela-server:latest

# Infrastructure
cd terraform && terraform apply
```
