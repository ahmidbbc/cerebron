# MVP Architecture

## Goal

Build a service that detects meaningful operational signals, correlates them with code ownership and recent changes, and reports only high-value events to Slack.

## MVP Scope

- runtime mode:
  - `test`: include all non-production environments
  - `prod`: include only production environments
- providers:
  - Datadog for current monitors and alert events
  - Elasticsearch for log bursts and new error patterns
  - Gerrit as the primary SCM provider
  - GitHub as a secondary SCM provider for frontend repositories
  - Slack for incident reporting
- no automated remediation
- no Jira integration yet
- no database introspection yet

## Core Flow

1. Poll or receive signals from Datadog monitors, Datadog alert events, and Elasticsearch.
2. Normalize all external data into one event model.
3. Apply the runtime environment policy and correlate events by service, environment, time window, and fingerprint.
4. Enrich the incident candidate with recent code changes and team ownership.
5. Score the event to decide if it is worth publishing.
6. Post a single concise report to Slack and update the same thread when the incident evolves.

The current scaffold exposes a manual HTTP trigger to run the monitoring use case end to end before real schedulers and provider adapters are added.

## Main Modules

- `handler`: inbound adapters such as HTTP or webhook handlers
- `usecase`: application workflows and orchestration rules
- `domain`: core entities shared across the service
- `port`: outbound contracts for providers and persistence
- `collector`: fetches alerts, log signals, and SCM metadata
- `correlator`: groups signals that likely describe the same incident
- `ownership`: resolves team, repository, channel, and runbook
- `reasoning`: prepares a reduced context for the future LLM layer
- `reporting`: anti-spam rules, thresholds, and Slack formatting

## Data Contracts

The normalized event contract should keep these fields stable across all providers:

- `source`
- `service`
- `environment`
- `severity`
- `occurred_at`
- `fingerprint`
- `summary`
- `attributes`
- `change_refs`
- `owner_team`

## Initial Persistence

- `PostgreSQL`:
  - normalized events
  - incident records
  - ownership mappings
  - connector configuration
- `Redis`:
  - deduplication keys
  - cooldown windows
  - scheduler locks

## Why Deterministic First

The first objective is trust, not novelty. If the service cannot correlate Datadog, Elasticsearch, and SCM metadata reliably, the LLM will only produce polished noise. The reasoning layer should therefore consume already-filtered incident candidates rather than raw operational streams.

## Next Delivery Steps

1. implement provider adapters for Datadog, Elasticsearch, Gerrit, GitHub, and Slack
2. implement the first monitoring use case that consumes outbound ports
3. add correlation rules over a bounded time window
4. persist incidents and deduplication state
5. integrate the first controlled LLM prompt for root-cause hypotheses
