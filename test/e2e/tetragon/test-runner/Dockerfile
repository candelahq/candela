# Build-stage: compile the Go test runner against the candela module.
# golang:1.27.4-alpine
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /tetragon-test-runner ./test/e2e/tetragon/test-runner

# Run-stage: minimal image with the test binary.
# alpine:3.24
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache ca-certificates

COPY --from=builder /tetragon-test-runner /usr/local/bin/tetragon-test-runner

ENTRYPOINT ["tetragon-test-runner"]
