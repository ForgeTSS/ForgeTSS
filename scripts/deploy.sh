#!/bin/bash
# scripts/deploy.sh — build and deploy ForgeTSS to a remote host via Docker.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

HOST="${1:-}"
ENVIRONMENT="${2:-testnet}"

if [ -z "$HOST" ]; then
  echo "Usage: $0 <host> [staging|production]"
  echo ""
  echo "Deploys ForgeTSS to the given host using docker compose."
  echo ""
  echo "Examples:"
  echo "  $0 forge-test.example.com"
  echo "  $0 forge-prod.example.com production"
  exit 1
fi

echo "Deploying ForgeTSS to $HOST ($ENVIRONMENT)..."

# Build the image
echo "Building Docker image..."
cd "$PROJECT_DIR"
docker build -t forge-tss:latest .

# Copy image to remote host
echo "Copying image to $HOST..."
docker save forge-tss:latest | ssh "$HOST" "docker load"

# Generate compose file with environment-specific config
cat > /tmp/docker-compose.deploy.yml <<EOF
services:
  forge-tss:
    image: forge-tss:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: "\${DATABASE_URL}"
      STELLAR_NETWORK: "${ENVIRONMENT}"
      HORIZON_URL: "\${HORIZON_URL:-https://horizon-testnet.stellar.org}"
      SOROBAN_RPC_URL: "\${SOROBAN_RPC_URL:-https://soroban-testnet.stellar.org}"
      MAX_RETRIES: "\${MAX_RETRIES:-5}"
      API_KEYS: "\${API_KEYS}"
EOF

# Deploy
echo "Deploying on $HOST..."
ssh "$HOST" "mkdir -p ~/.forge-tss && cp /dev/stdin ~/.forge-tss/docker-compose.yml" <<EOF
services:
  forge-tss:
    image: forge-tss:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: "\${DATABASE_URL}"
      STELLAR_NETWORK: "${ENVIRONMENT}"
      HORIZON_URL: "\${HORIZON_URL:-https://horizon-testnet.stellar.org}"
      SOROBAN_RPC_URL: "\${SOROBAN_RPC_URL:-https://soroban-testnet.stellar.org}"
      MAX_RETRIES: "\${MAX_RETRIES:-5}"
      API_KEYS: "\${API_KEYS}"
EOF

ssh "$HOST" "cd ~/.forge-tss && docker compose up -d"

echo ""
echo "Deploy complete. Check status with:"
echo "  ssh $HOST 'docker compose -f ~/.forge-tss/docker-compose.yml ps'"
echo "  ssh $HOST 'docker compose -f ~/.forge-tss/docker-compose.yml logs -f'"
