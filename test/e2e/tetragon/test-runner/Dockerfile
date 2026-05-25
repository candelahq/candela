# Build-stage: compile the Go test runner against the candela module.
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /tetragon-test-runner ./test/e2e/tetragon/test-runner

# Run-stage: minimal image with the test binary.
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /tetragon-test-runner /usr/local/bin/tetragon-test-runner

ENTRYPOINT ["tetragon-test-runner"]
