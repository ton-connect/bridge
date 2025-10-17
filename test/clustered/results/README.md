# Test Results Directory

This directory contains all test results from Valkey cluster scalability tests.

## Structure

- `cluster-3-nodes/` - Results from 2-node cluster tests
  - `summary-write-heavy.json` - k6 summary for write-heavy scenario
  - `summary-read-heavy.json` - k6 summary for read-heavy scenario
  - `summary-mixed.json` - k6 summary for mixed scenario
  - `test-metadata-*.txt` - Test configuration metadata

- `cluster-6-nodes/` - Results from 4-node cluster tests
  - `summary-write-heavy.json` - k6 summary for write-heavy scenario
  - `summary-read-heavy.json` - k6 summary for read-heavy scenario
  - `summary-mixed.json` - k6 summary for mixed scenario
  - `test-metadata-*.txt` - Test configuration metadata

- `charts/` - Generated comparison charts (PNG format)
  - `p95_latency_comparison.png`
  - `error_rate_comparison.png`
  - `delivery_latency_comparison.png`

- `bridge-stats/` - Bridge resource usage data
  - Docker stats snapshots
  - CPU/memory usage logs

## Usage

Results are automatically saved here when running tests via:
- `make test-3node-*`
- `make test-6node-*`

