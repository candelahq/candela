# Tetragon Integration Test

End-to-end integration test that validates the Candela Tetragon audit pipeline
against a **real Tetragon agent** running in a Docker container.

## What It Tests

| Test | Description |
|------|-------------|
| **Event Schema Fidelity** | Generates `tcp_connect` kprobe events for port 443, then validates that Tetragon events arrive via gRPC with correct `FunctionName`, `DstPort`, `Action`, `Binary`, and `Timestamp` fields. |
| **Pipeline Health** | Confirms the `tetragonaudit.Pipeline` reports `Connected=true` and `Processed>0` after receiving real eBPF events. |
| **Pipeline Stats** | Validates that the pipeline's processing counters are consistent. |

## Architecture

```
┌─────────────────────┐     gRPC :54321     ┌──────────────────────┐
│   Tetragon Agent    │ ◄────────────────── │    Test Runner (Go)  │
│  (privileged, BPF)  │                     │   uses tetragonaudit │
│                     │  eBPF kprobe events  │   package directly   │
│  TracingPolicy:     │ ──────────────────► │                      │
│  tcp_connect :443   │                     │  1. Start pipeline   │
│  action: Post       │                     │  2. Dial :443 targets│
└─────────────────────┘                     │  3. Assert events    │
                                            └──────────────────────┘
```

## Running Locally

> **Requires**: Docker with privileged container support and a Linux kernel
> with BPF support. **Will not work on macOS Docker Desktop** (no real BPF).

```bash
docker compose -f test/e2e/tetragon/docker-compose.yml up \
  --build --abort-on-container-exit --exit-code-from test-runner
```

### On a Linux VM or CI runner:

```bash
# Full run with cleanup:
docker compose -f test/e2e/tetragon/docker-compose.yml up \
  --build --abort-on-container-exit --exit-code-from test-runner && \
docker compose -f test/e2e/tetragon/docker-compose.yml down -v
```

## CI

The integration test runs automatically via `.github/workflows/tetragon-integration.yml`:

- **Triggers**: Manual (`workflow_dispatch`), pushes to `main`, PRs modifying
  `pkg/tetragonaudit/` or `test/e2e/tetragon/`.
- **Runner**: `ubuntu-latest` (has BPF support).
- **Artifacts**: Tetragon agent and test runner logs are uploaded on failure.

## TracingPolicy

The test uses a simplified TracingPolicy (`tracingpolicy.yaml`) that matches
`tcp_connect` to port 443 with `Post` action (log-only). This mirrors the
production Helm-generated policy but avoids `Sigkill` to keep the test runner
alive.

## Relationship to Unit Tests

The existing mock-based e2e tests in `pkg/tetragonaudit/grpc_e2e_test.go`
cover the transport layer comprehensively (7 tests). This integration test
complements them by validating:

- **Real eBPF event generation** from kernel hooks (not mocked)
- **Real protobuf-to-JSON** event format from Tetragon's actual export API
- **Container-scoped** event filtering behavior
