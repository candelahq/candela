# Build-stage: compile the Go test runner against the candela module.
# golang:1.26-alpine
FROM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /tetragon-test-runner ./test/e2e/tetragon/test-runner

# Run-stage: minimal image with the test binary.
# alpine:3.21
FROM alpine:3.21@sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507

RUN apk add --no-cache ca-certificates

COPY --from=builder /tetragon-test-runner /usr/local/bin/tetragon-test-runner

ENTRYPOINT ["tetragon-test-runner"]
