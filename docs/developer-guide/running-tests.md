# Running Tests

The test suite covers the store (`store/store_test.go`), the submission engine (`submission/engine_test.go`), the RPC router (`rpc/router_test.go`), the client (`pkg/client/client_test.go`), and the API handlers (`api/handlers_test.go`).

## Basic command

```bash
go test ./...
```

This runs all packages. If a package requires a live database connection and one is not available, those tests will fail rather than skip — the design assumes a Postgres instance is available when running the full suite.

## With a test database

The `README.md` recommends:

```bash
docker compose up postgres -d
TEST_DATABASE_URL="postgresql://forge:forgepass@localhost:5432/forge?sslmode=disable" go test -count=1 ./...
```

Make sure the database is ready before the test runs (`pg_isready`). The `-count=1` disables Go's test caching, which is useful when the database state changes between runs (migrations, inserted test data, leaked rows).

## What the tests cover

- `store` — database interactions, migrations, channel account leasing with `SELECT ... FOR UPDATE SKIP LOCKED`
- `submission` — retry logic, fee-bump behavior, engine polling
- `rpc` — endpoint failover, router selection between Horizon and Soroban
- `api` — handler validation, authentication branch behavior (`API_KEYS` empty vs. set)
- `pkg/client` — SDK request formatting, status parsing

There is no separate integration test suite that runs against a live Stellar testnet endpoint. Unit tests mock or stub external dependencies. If you change endpoint failover logic or retry timing, add or update the corresponding `rpc/router_test.go` or `submission/engine_test.go` cases.

## Continuous integration

`.github/workflows/ci.yml` defines the build pipeline. It builds `go build ./...`, runs `go test ./...`, and expects both to pass. Any documentation change that touches code behavior (e.g., updating variable names in examples) should keep the tests passing; documentation-only changes do not trigger build failures on their own.
