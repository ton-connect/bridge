# Docker Benchmark (Docker + k6)

This directory contains a simple Docker-based setup for running bridge benchmark tests using k6 with SSE support.

For full benchmark documentation (architecture, metrics, configuration details), see the main guide in `README.md`.

Benchmark execution is managed by the `run_k6_docker.sh` script, which wraps `docker run` and can launch one or multiple k6 instances.

## Quick Start

### 1. Build the image

From the `benchmark` directory:

```bash
cd benchmark
docker build -t k6-benchmark:latest .
```

### 2. Single instance

```bash
cd benchmark
./run_k6_docker.sh
```

This:
- runs one k6 instance in Docker
- connects to `http://localhost:8081/bridge` by default
- saves results to `benchmark/results/summary-0.json`

## Multiple Instances

`run_k6_docker.sh` supports multiple instances via `TOTAL_INSTANCES`. Each instance gets its own `CURRENT_INSTANCE` (0, 1, 2, …) and its own summary file.

```bash
cd benchmark
TOTAL_INSTANCES=3 ./run_k6_docker.sh
```

This will run 3 parallel containers with:
- `TOTAL_INSTANCES=3`
- `CURRENT_INSTANCE=0`, `1`, `2`
- results saved as:
  - `results/summary-0.json`
  - `results/summary-1.json`
  - `results/summary-2.json`

## Configuration

All configuration is done via environment variables passed to `run_k6_docker.sh`. See `README.md` and `bridge_test.js` for full details.

### Common Variables

- `BRIDGE_URL` – Bridge server URL (default: `http://localhost:8081/bridge`)
- `AUTH_TOKEN` – Auth token for bypassing rate limits (default: `test-token`)
- `SSE_VUS` – Number of SSE listener virtual users (default: `1000` in `run_k6_docker.sh`)
- `SEND_RATE` – Target messages per second (default: `10` in `run_k6_docker.sh`)
- `TOTAL_INSTANCES` – Total number of test instances (default: `1`)
- `ID_SPACE_OFFSET` – Offset for ID space when running multiple sets of tests

### Timing Variables

- `SSE_RAMP_UP`, `SSE_HOLD`, `SSE_RAMP_DOWN`, `SSE_DELAY`
- `SENDER_RAMP_UP`, `SENDER_HOLD`, `SENDER_RAMP_DOWN`, `SENDER_DELAY`

All support `s`, `m`, `h` suffixes (e.g. `30s`, `5m`, `1h`).

## Results

Results are saved to:

- `benchmark/results/summary-{INSTANCE}.json`

Example:

```bash
cd benchmark
cat results/summary-0.json | jq
```

## Examples

### Quick Test (single instance)

```bash
cd benchmark
SSE_VUS=100 SEND_RATE=10 \
SSE_HOLD=30s SENDER_HOLD=30s \
./run_k6_docker.sh
```

### High Load Test with 5 Instances

```bash
cd benchmark
TOTAL_INSTANCES=5 SSE_VUS=5000 SEND_RATE=50 \
SSE_HOLD=30m SENDER_HOLD=30m \
./run_k6_docker.sh
```
