# opentelemetry-collector-components

Personal collection of custom [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector)
components (receivers, processors, connectors, exporters).

Layout mirrors upstream [opentelemetry-collector-contrib](https://github.com/open-telemetry/opentelemetry-collector-contrib):
each component lives in its own directory under `receivers/`, `processors/`, `connectors/`, or `exporters/`,
and is its own Go module. The repo root uses a `go.work` workspace to tie the modules together for local
development.

## Components

| Component | Type | Description |
| --- | --- | --- |
| [`exporters/notificationexporter`](./exporters/notificationexporter) | exporter (logs) | Renders log records through a user-defined template and POSTs the result to an arbitrary HTTP endpoint (Slack, Discord, generic webhooks, ...). |

## Development

```sh
go work sync
go build ./...
go test ./...
```

Collector core (`go.opentelemetry.io/collector/*`) and `pkg/ottl` dependency versions are pinned per-component
to match a specific `opentelemetry-collector-contrib` release, so that components can be built into a custom
distribution alongside upstream contrib components at that same release without version skew.
