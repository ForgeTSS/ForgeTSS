#!/bin/bash
# scripts/setup-testnet.sh — bootstrap channel accounts on Stellar testnet.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

DATABASE_URL="${DATABASE_URL:-postgresql://forge:forgepass@localhost:5432/forge?sslmode=disable}"
MASTER_SEED="${MASTER_SEED:-}"
REFILL_COUNT="${REFILL_COUNT:-5}"

if [ -z "$MASTER_SEED" ]; then
  echo "ERROR: MASTER_SEED is not set"
  echo "Generate one with: go run github.com/stellar/go-stellar-sdk/keypair/Random"
  exit 1
fi

echo "Setting up ForgeTSS testnet environment..."
echo "  DATABASE_URL: $DATABASE_URL (masking credential)"
echo "  REFILL_COUNT: $REFILL_COUNT"
echo ""

# Run migrations
echo "Running migrations..."
cd "$PROJECT_DIR"
go run ./cmd/forgetss migrate --db-url "$DATABASE_URL"

echo ""
echo "Setting up $REFILL_COUNT channel accounts..."
go run ./cmd/forgetss setup-channels --db-url "$DATABASE_URL" --master-seed "$MASTER_SEED" --count "$REFILL_COUNT"

echo ""
echo "Done. Channel accounts ready on Stellar testnet."
