TON Connect bridge events compliance plan

- Re-read `tonmetrics/specification.md` to extract a checklist of required events, mandatory fields, allowed values, timing/batching limits, and header requirements.
- Map where bridge-side code emits TON Connect analytics (likely `tonmetrics`, `internal/*`, `cmd/bridge*`) and outline the data flow from event trigger to payload sent to `/events` (including tracing IDs, environment, network id).
- For each specified bridge event, locate the instrumentation, confirm it fires at the right lifecycle moment, and compare the payload schema (field names, types, required values, enums) with the spec.
- Verify global constraints: UUID/UUIDv7 generation rules, 24h freshness, batch sizing (<=100 events, <=1MB), `X-Client-Timestamp` handling, and `client_environment/subsystem` values.
- Review error/edge handling (validation failures, missing user_id behavior, unsupported network ids) and ensure deduplication/event ID logic aligns with the requirements.
- Note deviations, ambiguities, or missing coverage and record findings in `result.md`; propose fixes/tests if mismatches are found.
