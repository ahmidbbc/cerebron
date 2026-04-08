# Cerebron

Cerebron is a Docker-ready Go service that correlates operational signals from Datadog, Elasticsearch, Gerrit, GitHub, and Slack before escalating important incidents to a dedicated Slack channel.

The first milestone is a deterministic MVP:

- ingest Datadog monitor alerts, Datadog alert events, Datadog error tracking issues, and log spikes from Datadog or Elasticsearch
- map an event to a service and environment
- inspect recent changes and ownership from Gerrit and GitHub
- decide whether the event is important enough to report
- publish a concise report to Slack without spamming

The AI layer comes after the correlation pipeline is stable.

## Repository Layout

- `cmd/cerebron-api`: HTTP API entrypoint
- `internal/app`: application bootstrap and dependency wiring
- `internal/config`: environment-based configuration
- `internal/domain`: core entities and value objects
- `internal/usecase`: application use cases
- `internal/handler`: inbound adapters such as HTTP handlers
- `internal/port`: outbound contracts for providers and repositories
- `docs`: MVP architecture notes

## Local Development

Start dependencies:

```bash
docker compose up -d
```

Run the service:

```bash
go run ./cmd/cerebron-api
```

Default environment policy:

- `APP_MODE=test`: keep all signals except `prod` and `production`
- `APP_MODE=prod`: keep only `prod` and `production`
- `OBSERVED_ENVS` is optional and acts as an explicit override list
- request-level `environments` filters remain optional and support environment families such as `qa` matching `qa3`

Trigger a manual monitoring run:

```bash
curl -X POST http://localhost:8080/api/v1/monitoring/signals/run \
  -H 'Content-Type: application/json' \
  -d '{"since":"15m"}'
```

The response includes aggregate counters such as `alert_events`, `log_events`, `collected_events`, `correlated_events`, `reportable`, and `score`.

Inspect provider filtering decisions during a manual test:

```bash
curl -X POST http://localhost:8080/api/v1/monitoring/signals/run \
  -H 'Content-Type: application/json' \
  -d '{"since":"240h","environments":["preprod"],"debug":true}'
```

Enable Datadog alert collection with:

```bash
export DATADOG_ENABLED=true
export DATADOG_BASE_URL=https://api.datadoghq.eu
export DATADOG_API_KEY=...
export DATADOG_APP_KEY=...
export DATADOG_MONITOR_TAGS=env:qa3
export APP_MODE=test
```

Enable Datadog Error Tracking collection with:

```bash
export DATADOG_ERROR_TRACKING_ENABLED=true
export DATADOG_ERROR_TRACKING_BASE_URL=https://api.datadoghq.eu
export DATADOG_ERROR_TRACKING_TRACK=trace
export DATADOG_ERROR_TRACKING_QUERY='service:presence-api env:preprod'
```

`DATADOG_ERROR_TRACKING_QUERY` is required when Datadog Error Tracking is enabled. For deterministic environment filtering, keep an explicit `env:...` filter in that query because the Error Tracking issue search response does not expose a normalized environment field directly.

Enable Elasticsearch log collection with:

```bash
export ELASTIC_ENABLED=true
export ELASTIC_BASE_URL=https://your-elastic-cluster.example.com
export ELASTIC_TOKEN='ApiKey ...'
export ELASTIC_INDEX_PATTERN=logs-*
export ELASTIC_ENVIRONMENT_FIELDS=k8s-namespace,service.environment,environment,env
```

`ELASTIC_TOKEN` is optional when the cluster is reachable without HTTP authentication, for example from a pod running inside the target Kubernetes network.

`ELASTIC_ENVIRONMENT_FIELDS` defines which log fields should be used both to map the normalized environment and to prefilter `_search` requests when a monitoring run targets a specific environment family such as `qa`, `preprod`, or `prod`.

The Elasticsearch adapter currently performs a bounded `_search` over the configured time window and maps common log fields such as `@timestamp`, `message`, `error`, `log.level`, `status`, `application`, `service.name`, `k8s-namespace`, `service.environment`, `env`, and `team` into `domain.Event`.

For correlation, the current deterministic MVP gives priority to:

- service identity
- environment family
- HTTP status code or status class (`4xx`, `5xx`)
- time proximity
- route as an optional strengthening signal

Datadog Error Tracking issues are normalized into the same `domain.Event` model as monitor alerts. In the current stage, the adapter maps issue `service`, `error_type`, `error_message`, `last_seen`, and team owners, while environment is derived from the configured Error Tracking query.

Run checks:

```bash
gofmt -w .
go test ./...
```

## Current Status

This repository currently contains the service skeleton, core interfaces, local dependencies, a manual monitoring trigger endpoint, and the first architecture draft for the MVP.
