# Build-stage: compile the Go test runner against the candela module.
# golang:1.26-alpine
FROM golang:1.26-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /tetragon-test-runner ./test/e2e/tetragon/test-runner

# Run-stage: minimal image with the test binary.
# alpine:3.21
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache ca-certificates

COPY --from=builder /tetragon-test-runner /usr/local/bin/tetragon-test-runner

ENTRYPOINT ["tetragon-test-runner"]
