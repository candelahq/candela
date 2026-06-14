# ADR 003: Database-Agnostic Storage Interface

**Status:** Accepted
**Date:** 2024-06-14

## Context

The catalog store could use proto-generated types directly, coupling storage to the protobuf schema. This would make backend implementations depend on proto codegen.

## Decision

Use plain Go structs (`catalog.Entry`) in the storage layer. The ConnectRPC handler converts between proto and Go types at the boundary.

## Consequences

- **No proto dependency in `pkg/catalog/`**: backend implementations are pure Go
- **Testability**: tests don't need proto codegen tooling
- **Flexibility**: the Go struct can have fields not in the proto (e.g., internal metadata)
- **Cost**: manual conversion code in the handler (small, ~20 lines)
