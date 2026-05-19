#!/bin/sh
# Generate self-signed certs for the mock upstream.
set -e

CERT_DIR="$(dirname "$0")/certs"
mkdir -p "$CERT_DIR"

openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$CERT_DIR/server.key" \
  -out "$CERT_DIR/server.crt" \
  -days 365 \
  -subj "/CN=api.openai.com" \
  -addext "subjectAltName=DNS:api.openai.com,DNS:api.anthropic.com,DNS:*.aiplatform.googleapis.com" \
  2>/dev/null

echo "✅ Self-signed certs generated in $CERT_DIR"
