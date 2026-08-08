# Grafana dashboard

`github-platform-operator.json` is an importable Grafana dashboard for the
operator's Prometheus metrics.

It combines the custom GitHub API metrics exported by the operator with the
standard metrics already exposed by `controller-runtime`, the Go runtime and
the process collector.

## Prerequisite

Prometheus must scrape the operator `/metrics` endpoint. The project already
ships the metrics Service and a ServiceMonitor example under
[`config/prometheus`](../../config/prometheus).

When secure metrics are enabled, the Prometheus service account must be allowed
to read the authenticated `/metrics` endpoint. See
[`docs/operations.md`](../../docs/operations.md#metrics-and-grafana) for the
operational notes.

## Import

In Grafana, use **Dashboards → New → Import**, upload
`github-platform-operator.json`, and select the Prometheus data source.

The dashboard includes:

- GitHub API request volume and HTTP status codes
- GitHub API p95 latency
- transport errors
- rate-limit remaining/limit/reset state
- primary and secondary rate-limit events
- shared rate-limit gate blocking time
- controller reconciliation rate, errors and p95 duration
- workqueue depth
- process CPU, resident memory and Go goroutines

The dashboard intentionally avoids repository, organization, username and token
labels to keep metric cardinality bounded and to avoid exposing sensitive
identifiers through Prometheus labels.
