# 🖥️ Candela UI — Next.js Web Dashboard

The Candela web interface is a **Next.js 16** application with a dark-themed, glassmorphism-inspired design. It communicates with the Go backend via **ConnectRPC v2** and uses **Firebase Auth** for user authentication.

## Quick Start

```bash
cd ui
pnpm install       # install dependencies (pnpm is in the Nix shell)
pnpm run dev       # start dev server → http://localhost:3000
```

> [!NOTE]
> The backend must be running on `:8181` for the UI to function. Start it with:
> ```bash
> nix develop -c go run ./cmd/candela-server
> ```

---

## Architecture

```
┌─────────────────────────────────────────┐
│               Next.js App               │
│  ┌──────────────────────────────────┐   │
│  │  App Router (src/app/)           │   │
│  │  ├── page.tsx (Dashboard)        │   │
│  │  ├── traces/ (Trace list + detail│   │
│  │  ├── costs/ (Cost analytics)     │   │
│  │  ├── usage/ (Usage metrics)      │   │
│  │  ├── projects/ (Project mgmt)    │   │
│  │  ├── settings/                   │   │
│  │  ├── login/                      │   │
│  │  └── admin/ (Users, Budgets,     │   │
│  │             Audit log)            │   │
│  ├──────────────────────────────────┤   │
│  │  Hooks (src/hooks/)              │   │
│  │  ├── useDashboard     useTraces  │   │
│  │  ├── useCurrentUser   useUsage   │   │
│  │  ├── useCosts         useTrace   │   │
│  │  ├── useLeaderboard              │   │
│  │  └── useProtoValidation          │   │
│  ├──────────────────────────────────┤   │
│  │  Components (src/components/)    │   │
│  │  ├── AppShell    Sidebar         │   │
│  │  ├── AuthGuard   AuthProvider    │   │
│  │  ├── BudgetGauge BudgetAlert     │   │
│  │  ├── AreaChart   Tooltip         │   │
│  │  └── ErrorBanner SkeletonCard    │   │
│  ├──────────────────────────────────┤   │
│  │  Lib (src/lib/)                  │   │
│  │  ├── connect.ts  (transport)     │   │
│  │  ├── firebase.ts (auth client)   │   │
│  │  ├── api.ts      (fetch wrapper) │   │
│  │  └── constants.ts                │   │
│  ├──────────────────────────────────┤   │
│  │  Generated (src/gen/)            │   │
│  │  └── TS proto stubs (ConnectRPC) │   │
│  └──────────────────────────────────┘   │
│                  │                      │
│       ConnectRPC v2 (HTTP/JSON)         │
│                  │                      │
└──────────────────│──────────────────────┘
                   ▼
         Go Backend (:8181)
```

---

## Pages & Routes

| Route | Page | Description |
|-------|------|-------------|
| `/` | Dashboard | Summary cards (traces, tokens, cost, latency), time-series charts, recent traces table |
| `/traces` | Trace List | Filterable, paginated trace explorer with search |
| `/traces/[id]` | Trace Detail | Span waterfall with timing, tokens, cost per span; expandable prompt/completion views with JSON formatting |
| `/costs` | Cost Analytics | Cost breakdown by model, provider, and time period |
| `/usage` | Usage Metrics | Token usage, model breakdown, user leaderboard (team mode) |
| `/projects` | Projects | Project CRUD, API key management |
| `/settings` | Settings | Backend connection status, storage info |
| `/login` | Login | Firebase Google Sign-In |
| `/admin/users` | User Management | User CRUD, role assignment, status management |
| `/admin/budgets` | Budgets | Budget enforcement explainer, waterfall visualization |
| `/admin/audit` | Audit Log | Filterable admin action timeline |

---

## Key Components

### Data Fetching Hooks

All data fetching uses ConnectRPC client stubs with custom React hooks:

| Hook | Service | Provides |
|------|---------|----------|
| `useDashboard` | `DashboardService` | Summary stats, time-series data, recent traces |
| `useTraces` | `TraceService` | Paginated trace list with filters |
| `useTrace` | `TraceService` | Single trace with all spans |
| `useCosts` | `DashboardService` | Cost analytics by model/provider |
| `useUsage` | `DashboardService` | Token usage metrics |
| `useLeaderboard` | `DashboardService` | Per-user usage ranking (team mode) |
| `useCurrentUser` | `UserService` | Current user profile, budget, grants |
| `useProtoValidation` | N/A | Client-side proto validation via `@bufbuild/protovalidate` |

### Authentication Components

| Component | Purpose |
|-----------|---------|
| `AuthProvider` | Wraps the app with Firebase Auth context; handles `onAuthStateChanged` |
| `AuthGuard` | Redirects unauthenticated users to `/login`; shows loading state |
| `AppShell` | Layout wrapper with sidebar, handles auth state |

### UI Components

| Component | Purpose |
|-----------|---------|
| `Sidebar` | Navigation with active route highlighting; shows user info and budget |
| `AreaChart` | SVG-based time-series chart with hover tooltips |
| `BudgetGauge` | Circular progress indicator for budget utilization |
| `BudgetAlert` | Warning banner when budget thresholds are crossed |
| `ErrorBanner` | Backend offline state with recovery instructions |
| `SkeletonCard` | Loading placeholder with shimmer animation |
| `TimeRangeSelector` | 24h / 7d / 30d range picker |
| `Tooltip` | Hover tooltip with arrow positioning |

---

## Proto Generation

The UI uses TypeScript proto stubs generated by Buf:

```bash
# From the repo root
cd proto && buf generate
```

This generates files into `gen/ts/candela/`. The CI pipeline copies them:

```bash
cp -r gen/ts/candela/* ui/src/gen/
rm -f ui/src/gen/types/bq_span_pb.ts  # BigQuery schema — server-only
```

---

## Styling

The UI uses **vanilla CSS** with a custom design system defined in `src/app/globals.css` (~42K lines). Key design tokens:

- **Dark theme** with glassmorphism effects
- CSS custom properties for all colors, spacing, and typography
- Responsive grid layouts
- Smooth animations with `@keyframes` (fade-in, slide-in, shimmer)
- Zero external CSS frameworks (no Tailwind)

---

## Testing

### E2E Tests (Playwright)

```bash
pnpm run test:e2e       # headless
pnpm run test:e2e:ui    # interactive UI mode
```

Three test suites:

| Suite | File | Tests | Coverage |
|-------|------|-------|----------|
| **App** | `e2e/app.spec.ts` | 20+ | Dashboard, traces, costs, usage, projects, settings, offline state |
| **Admin** | `e2e/admin.spec.ts` | 5+ | User management, budgets, audit log, access control |
| **Team Mode** | `e2e/team_mode.spec.ts` | 5+ | Leaderboard, per-user filtering, budget alerts |

### Linting

```bash
pnpm run lint           # ESLint
```

### Type Checking

```bash
pnpm run build          # includes TypeScript type-check
```

---

## Build & Deployment

### Development

```bash
pnpm run dev            # Next.js dev server with hot reload
```

### Production Build

```bash
pnpm run build          # generates .next/standalone for containerized deployment
```

### Container

The UI is built as part of the combined Docker image (`deploy/Dockerfile.server`). The Next.js standalone output is served by `node server.js` — see [docs/deployment.md](../docs/deployment.md).

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEXT_PUBLIC_API_URL` | `""` | Backend URL. Empty = same-origin (production). Set `http://localhost:8181` for local dev with separate backend. |
| `NEXT_PUBLIC_FIREBASE_*` | _(required)_ | Firebase client config — see [docs/env-vars.md](../docs/env-vars.md) |
| `BACKEND_URL` | `http://localhost:8181` | Backend URL for Next.js rewrites (server-side) |

---

## Offline Behavior

The UI gracefully handles a missing backend:
- Dashboard shows an `ErrorBanner` with recovery instructions
- All hooks return empty/default data instead of crashing
- Real-time polling pauses when errors are detected
- Reconnects automatically when the backend becomes available
