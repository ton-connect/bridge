# TON Metrics Code Generation

This directory owns the Go models used to send TON analytics bridge events.

## Prerequisites
- Go 1.24 or newer (for installing `oapi-codegen`)
- `make`, `jq`

## Usage
Run `make generate` from this folder to regenerate `bridge_events.gen.go`. The recipe:
1. Downloads the latest swagger specification from `analytics.ton.org`.
2. Reduces it to a minimal OpenAPI 3 document that only keeps `Bridge*` schemas via `jq`.
3. Invokes a pinned version of `oapi-codegen` to emit Go structs into `bridge_events.gen.go`.
4. Formats the result with `gofmt`.

Temporary swagger artifacts live under `.tmp/`.
