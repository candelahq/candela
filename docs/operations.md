# 🛡️ Operations Runbook

This runbook covers day-to-day operations, monitoring, incident response, and maintenance for a production Candela deployment on Google Cloud.

## Deployment

### Manual Deploy to Cloud Run

```bash
PROJECT=your-gcp-project
REGION=us-central1

# 1. Build and push the container image
gcloud builds submit --project $PROJECT \
  -f deploy/cloudbuild.yaml \
  --substitutions=\
_FIREBASE_API_KEY=AIza...,\
_FIREBASE_AUTH_DOMAIN=$PROJECT.firebaseapp.com,\
_FIREBASE_PROJECT_ID=$PROJECT,\
_FIREBASE_APP_ID=1:123:web:abc \
  .

# 2. Deploy to Cloud Run
gcloud run services update candela \
  --project $PROJECT \
  --region $REGION \
  --image $REGION-docker.pkg.dev/$PROJECT/candela/candela-server:latest
```

### Rolling Back

```bash
# List revisions
gcloud run revisions list --project $PROJECT --region $REGION --service candela

# Route 100% traffic to a previous revision
gcloud run services update-traffic candela \
  --project $PROJECT --region $REGION \
  --to-revisions=candela-00042-abc=100
```

### Environment Variables

Set production env vars on the Cloud Run service:

```bash
gcloud run services update candela \
  --project $PROJECT --region $REGION \
  --set-env-vars=\
CANDELA_STORAGE_BACKEND=bigquery,\
CANDELA_BQ_PROJECT=$PROJECT,\
CANDELA_FIRESTORE_PROJECT=$PROJECT,\
CANDELA_PROXY_ENABLED=true,\
CANDELA_VERTEX_PROJECT=$PROJECT,\
CANDELA_VERTEX_REGION=us-east5,\
CLOUD_RUN_URL=https://candela-xxx.a.run.app
```

See [docs/env-vars.md](env-vars.md) for the full reference.

---

## Health Checks

### Backend Health

```bash
# Local
curl http://localhost:8181/healthz

# Production (requires auth)
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  https://candela-xxx.a.run.app/healthz
```

Response:
```json
{"status": "ok"}        // healthy
{"status": "error", "detail": "..."} // storage unreachable
```

### Cloud Run Health

```bash
gcloud run services describe candela --project $PROJECT --region $REGION \
  --format='value(status.conditions)'
```

---

## Monitoring

### Key Metrics

| Metric | Where | Alert Threshold |
|--------|-------|----------------|
| Request latency (p99) | Cloud Run metrics | > 5s |
| Error rate (5xx) | Cloud Run metrics | > 5% |
| Container startup time | Cloud Run metrics | > 30s |
| BigQuery write errors | Application logs | Any |
| Auth failures | Application logs (`"all auth strategies failed"`) | > 10/min |
| Circuit breaker trips | Application logs (`"circuit breaker tripped"`) | Any |
| Budget thresholds | Application logs (`"🔔 budget alert"`) | At 80%, 90%, 100% |
| Span processor buffer full | Application logs (`"span processor buffer full"`) | Any |

### Log-Based Alerts

Create Cloud Logging alerts for critical events:

```bash
# Budget threshold alert
gcloud logging metrics create candela-budget-alert \
  --description="Candela budget threshold reached" \
  --log-filter='resource.type="cloud_run_revision"
    AND textPayload=~"budget alert"'

# Circuit breaker alert
gcloud logging metrics create candela-circuit-breaker \
  --description="Candela circuit breaker tripped" \
  --log-filter='resource.type="cloud_run_revision"
    AND textPayload=~"circuit breaker tripped"'
```

### Structured Logging

Candela uses `slog` with JSON output. Key log fields:

| Field | Description |
|-------|-------------|
| `provider` | LLM provider name |
| `model` | Model name |
| `tokens` | Total token count |
| `cost_usd` | Calculated cost |
| `latency` | Request duration |
| `user_id` | Authenticated user |
| `request_id` | Unique request ID (X-Request-ID) |

---

## BigQuery Operations

### Query Costs

```sql
-- Total cost by user, last 7 days
SELECT
  user_id,
  SUM(gen_ai_cost_usd) as total_cost,
  COUNT(*) as call_count,
  SUM(gen_ai_total_tokens) as total_tokens
FROM `candela.spans`
WHERE start_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY user_id
ORDER BY total_cost DESC
```

### Cost Optimization

| Optimization | Impact | How |
|-------------|--------|-----|
| Time partitioning | ~70% scan cost reduction | Already configured (`start_time`, DAY) |
| Clustering | ~50% for filtered queries | Already configured (`project_id`, `trace_id`) |
| Partition expiration | Storage savings | Set in Terraform: `expiration_ms` |
| BI Engine reservation | Sub-second dashboard queries | Enable in BigQuery console |

### Table Maintenance

```sql
-- Check table size
SELECT
  table_id,
  ROUND(size_bytes / 1e9, 2) as size_gb,
  row_count
FROM `candela.__TABLES__`
WHERE table_id = 'spans'
```

---

## Firestore Operations

### User Management

```bash
# List all users (via gcloud)
gcloud firestore documents list \
  --project=$PROJECT --database=candela \
  --collection-path=users

# Get a specific user
gcloud firestore documents get \
  --project=$PROJECT --database=candela \
  users/USER_ID
```

### Quotas

| Resource | Free Tier | Limit |
|----------|-----------|-------|
| Document reads | 50K/day | N/A (pay per use) |
| Document writes | 20K/day | N/A |
| Document deletes | 20K/day | N/A |
| Max document size | N/A | 1 MiB |

For Candela's usage pattern (user CRUD, budget checks), the free tier is usually sufficient for teams under 50 users.

---

## Incident Response

### Scenario: Backend Not Starting

1. Check Cloud Run logs: `gcloud run logs read --project $PROJECT --service candela`
2. Common causes:
   - Missing env vars → check `entrypoint.sh` substitution
   - Firestore connection failed → check project ID and IAM
   - BigQuery auth failed → check service account roles
3. Quick fix: set `CANDELA_DEV_MODE=true` to bypass auth (temporary!)

### Scenario: High Latency

1. Check if a specific provider is slow: filter logs by `provider`
2. Check circuit breaker state in logs
3. Check BigQuery slot usage (if using BQ as reader)
4. Check Cloud Run instance count (may need min-instances > 0)

### Scenario: Budget Not Enforcing

1. Check Firestore `budgets/{userId}` document
2. Verify `period_start` is in the current period
3. Check if grants are absorbing spend: inspect `grants/` collection
4. Check logs for deduction errors: `"failed to deduct spend"`

### Scenario: Proxy Returns 502

1. Check upstream provider status (OpenAI, Vertex AI, Anthropic)
2. Check circuit breaker state: look for `"circuit breaker tripped"` logs
3. Check ADC token refresh: look for `"failed to get ADC token"` logs
4. Verify `vertex_ai.project_id` and region in config

---

## Maintenance

### Updating Model Pricing

When LLM providers change prices:
1. Update `pkg/costcalc/calculator.go` → `loadDefaults()`
2. Run tests: `nix develop -c go test ./pkg/costcalc -v`
3. Deploy — pricing takes effect on next restart

### Terraform State

```bash
cd terraform

# Check current state
tofu state list

# Import existing resources
tofu import google_cloud_run_v2_service.candela projects/$PROJECT/locations/$REGION/services/candela

# Plan changes
tofu plan
```

### Database Migrations

- **DuckDB**: Schema is auto-provisioned. No manual migrations needed.
- **SQLite**: Schema is auto-provisioned.
- **BigQuery**: Schema is auto-provisioned. Column additions are backward-compatible. Removing columns requires table recreation.
- **Firestore**: Schema-less. Field additions are backward-compatible.

### Certificate/Domain Management

```bash
# Check domain mapping
gcloud run domain-mappings list --project $PROJECT --region $REGION

# Add Firebase authorized domain
# Must be done in Firebase Console → Authentication → Settings → Authorized Domains
```
