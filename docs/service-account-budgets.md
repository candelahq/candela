# Service Account Budget Enforcement

> **⚠️ BREAKING CHANGE (v0.XX)**: Service account budget enforcement is now enabled
> by default. SAs without a configured budget are auto-provisioned with a $10/day
> daily limit. Set `CANDELA_SA_DAILY_BUDGET_USD` to adjust, or set it to `0` to
> disable auto-provisioning (SAs without a budget will be blocked).

## Overview

Service accounts (SAs) are now subject to the same budget enforcement as human
users. When an SA makes its first proxy request and has no budget configured,
Candela automatically provisions a default daily budget. This prevents a
compromised SA from accumulating unbounded LLM costs.

## How It Works

1. **First request**: SA calls the proxy with no budget in Firestore
2. **Auto-provision**: Candela creates a `$10/day` daily budget for the SA
3. **Normal enforcement**: All subsequent requests go through the standard
   budget pipeline — CheckBudget → pending spend reservation → DeductSpend →
   threshold alerts → auto-disable at 100%

### What SAs Get for Free

Because SAs use the same budget infrastructure as users:

| Capability | Works for SAs |
|:---|:---|
| Daily budget with timezone-aware rollover | ✅ |
| 80% / 90% / 100% threshold alerts (Slack, webhook) | ✅ |
| Auto-disable (`soft_blocked`) when budget exhausted | ✅ |
| Pending spend TOCTOU protection | ✅ |
| Durable outbox retry on Firestore failure | ✅ |
| Grant waterfall (SAs can receive grants) | ✅ |
| Per-model daily limits (#721) | ✅ |
| Dashboard visibility | ✅ |

## Configuration

### Environment Variables

| Variable | Default | Description |
|:---|:---|:---|
| `CANDELA_SA_DAILY_BUDGET_USD` | `10.0` | Default daily budget for SAs without an explicit budget |

### Config File (`config.yaml`)

```yaml
service_accounts:
  default_daily_budget_usd: 10.0  # $10/day default
```

### Per-SA Budget Override

Admins can set a custom budget for any SA using the `SetBudget` RPC:

```bash
# Via grpcurl or the dashboard admin panel
candela admin set-budget \
  --user "candela-ci@my-project.iam.gserviceaccount.com" \
  --limit 100.0 \
  --period daily
```

Or via the dashboard: **Admin → Users → (select SA) → Set Budget**.

## Fail-Open Behavior

If auto-provisioning fails (e.g., Firestore is temporarily unavailable), the
request is **allowed through** and the error is logged. This preserves
availability — budget enforcement is a financial control, not a security gate.

The next request will retry auto-provisioning.

## Startup Warning

When SA budget enforcement is active, you'll see this log at startup:

```
⚠️  SA budget enforcement active — service accounts without a configured budget
will be auto-provisioned at the default daily limit.
Use SetBudget RPC or CANDELA_SA_DAILY_BUDGET_USD to adjust.
```

## Disabling Auto-Provisioning

To disable auto-provisioning (SAs without a budget will be blocked with 402):

```yaml
service_accounts:
  default_daily_budget_usd: 0
```

Or: `CANDELA_SA_DAILY_BUDGET_USD=0`

> **Note**: This does NOT disable SA budget enforcement — SAs are always subject
> to budget checks when a UserStore is configured (#736). Setting the default to 0
> only disables auto-provisioning. SAs can still be manually given budgets via
> `SetBudget`.
