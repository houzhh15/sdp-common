#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERT_DIR="$SCRIPT_DIR/../certs"

echo "Creating certificate directory..."
mkdir -p "$CERT_DIR"

echo "Generating certificates for SDP examples..."

# 1. Generate CA key and certificate
echo "1. Generating CA certificate..."
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
    -keyout "$CERT_DIR/ca-key.pem" \
    -out "$CERT_DIR/ca-cert.pem" \
    -subj "/CN=SDP-CA/O=SDP-Examples"

# 2. Generate Controller certificate
echo "2. Generating Controller certificate..."
openssl req -newkey rsa:4096 -nodes \
    -keyout "$CERT_DIR/controller-key.pem" \
    -out "$CERT_DIR/controller-req.pem" \
    -subj "/CN=localhost/O=Controller"

openssl x509 -req -in "$CERT_DIR/controller-req.pem" -days 365 \
    -CA "$CERT_DIR/ca-cert.pem" \
    -CAkey "$CERT_DIR/ca-key.pem" \
    -CAcreateserial \
    -out "$CERT_DIR/controller-cert.pem" \
    -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

# 3. Generate IH Client certificate
echo "3. Generating IH Client certificate..."
openssl req -newkey rsa:4096 -nodes \
    -keyout "$CERT_DIR/ih-client-key.pem" \
    -out "$CERT_DIR/ih-client-req.pem" \
    -subj "/CN=ih-client/O=IH-Client"

openssl x509 -req -in "$CERT_DIR/ih-client-req.pem" -days 365 \
    -CA "$CERT_DIR/ca-cert.pem" \
    -CAkey "$CERT_DIR/ca-key.pem" \
    -CAcreateserial \
    -out "$CERT_DIR/ih-client-cert.pem"

# 4. Generate AH Agent certificate
echo "4. Generating AH Agent certificate..."
openssl req -newkey rsa:4096 -nodes \
    -keyout "$CERT_DIR/ah-agent-key.pem" \
    -out "$CERT_DIR/ah-agent-req.pem" \
    -subj "/CN=ah-agent/O=AH-Agent"

openssl x509 -req -in "$CERT_DIR/ah-agent-req.pem" -days 365 \
    -CA "$CERT_DIR/ca-cert.pem" \
    -CAkey "$CERT_DIR/ca-key.pem" \
    -CAcreateserial \
    -out "$CERT_DIR/ah-agent-cert.pem"

# Clean up CSR files
rm -f "$CERT_DIR"/*.pem.req "$CERT_DIR"/*-req.pem

echo ""
echo "✅ Certificate generation complete!"
echo ""
echo "Generated files in $CERT_DIR:"
echo "  - ca-cert.pem, ca-key.pem (CA)"
echo "  - controller-cert.pem, controller-key.pem"
echo "  - ih-client-cert.pem, ih-client-key.pem"
echo "  - ah-agent-cert.pem, ah-agent-key.pem"
echo ""
echo "You can now run the examples:"
echo "  cd examples/controller && ./controller-example"
echo "  cd examples/ih-client && ./ih-client-example"
echo "  cd examples/ah-agent && ./ah-agent-example"
