# GitHub Platform Operator

GitHub Platform Operator is a Kubernetes operator for declaratively creating,
adopting and continuously reconciling GitHub repositories and their common
platform resources.

It lets platform teams manage GitHub through Kubernetes custom resources and
GitOps workflows instead of manual configuration or one-off automation
scripts.

## Features

- create repositories from scratch or templates, or safely adopt existing ones
- manage repository metadata, visibility, features, merge options and alerts
- protect branches, tags and pushes with repository rulesets
- manage organization members, teams and team memberships
- grant team and direct collaborator access to repositories
- manage GitHub environments
- synchronize Actions secrets and variables
- authenticate with personal access tokens or GitHub Apps
- continuously detect and reconcile configuration drift
- use explicit, non-destructive deletion policies

> [!NOTE]
> The API currently uses `v1alpha1`. It is suitable for controlled production
> use. Required migration steps are documented in the release notes.

## Managed resources

| Resource | Purpose |
|---|---|
| `GitHubProviderConfig` | Organization, API endpoint and authentication |
| `GitHubRepository` | Repository creation, adoption and configuration |
| `GitHubRepositoryRuleset` | Branch, tag and push rulesets |
| `GitHubOrganizationMember` | Organization membership and roles |
| `GitHubTeam` | Organization team creation and adoption |
| `GitHubTeamMembership` | Team membership and roles |
| `GitHubRepositoryTeamAccess` | Team access to repositories |
| `GitHubRepositoryCollaborator` | Direct collaborator access |
| `GitHubEnvironment` | Repository environments |
| `GitHubActionsSecret` | Repository, environment and organization secrets |
| `GitHubActionsVariable` | Repository, environment and organization variables |

`GitHubProviderConfig` is cluster-scoped. All other resources are namespaced
and use `github.k8sready.com/v1alpha1`.

See the
[custom resource documentation](https://github.com/pierinho13/github-platform-operator/blob/main/docs/resources.md)
for complete schemas, supported values, examples and reconciliation behavior.

## Install

The chart is published as an OCI artifact at:

```text
oci://ghcr.io/pierinho13/charts/github-platform-operator
```

Install the latest version:

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --namespace github-platform-operator-system \
  --create-namespace
```

For reproducible installations, add `--version <chart-version>` using a
version listed in Artifact Hub. The controller image defaults to:

```text
ghcr.io/pierinho13/github-platform-operator:<chart appVersion>
```

Verify the installation:

```bash
kubectl rollout status \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system

kubectl get crd | grep github.k8sready.com
```

## Quick start

Create a Kubernetes Secret containing a GitHub token:

```bash
kubectl create secret generic github-credentials \
  --namespace default \
  --from-literal=token="${GITHUB_TOKEN}"
```

Create a provider and a repository:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubProviderConfig
metadata:
  name: default
spec:
  organization: my-organization
  credentials:
    secretRef:
      namespace: default
      name: github-credentials
      key: token
---
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepository
metadata:
  name: example-repository
  namespace: default
spec:
  providerConfigRef: default
  name: example-repository
  visibility: private
  description: Repository managed by GitHub Platform Operator
  deletionPolicy: Orphan
```

Apply the resources and wait for reconciliation:

```bash
kubectl apply -f resources.yaml
kubectl wait \
  --for=condition=Ready \
  ghrepo/example-repository \
  --timeout=90s
```

If the repository already exists in the provider organization, the operator
adopts it. Optional fields that are omitted remain unmanaged and retain their
existing GitHub values.

Continue with the
[getting started guide](https://github.com/pierinho13/github-platform-operator/blob/main/docs/getting-started.md)
to configure GitHub App authentication, teams, rulesets and other resources.

## Common values

```yaml
replicaCount: 1

image:
  repository: ghcr.io/pierinho13/github-platform-operator
  tag: ""
  pullPolicy: IfNotPresent

leaderElection:
  enabled: true

metrics:
  enabled: true
  secure: true
  port: 8443
  serviceMonitor:
    enabled: false
    interval: 30s
    scrapeTimeout: 10s

networkPolicy:
  enabled: false

rbac:
  create: true
```

Show every available value:

```bash
helm show values \
  oci://ghcr.io/pierinho13/charts/github-platform-operator
```

The operator exposes standard `controller-runtime` metrics plus GitHub API and
rate-limit metrics. An importable Grafana dashboard is available in the source
repository at `dashboards/grafana/github-platform-operator.json`. See the
[operations guide](https://github.com/pierinho13/github-platform-operator/blob/main/docs/operations.md#metrics-and-grafana)
for scraping and dashboard details.

### Prometheus Operator ServiceMonitor

When the Prometheus Operator CRDs are installed, the chart can create a
`ServiceMonitor` for the metrics Service:

```yaml
metrics:
  enabled: true
  secure: true
  port: 8443
  serviceMonitor:
    enabled: true
    namespace: monitoring
    additionalLabels:
      release: kube-prometheus-stack
    interval: 30s
    scrapeTimeout: 10s
    prometheusServiceAccount:
      name: kube-prometheus-stack-prometheus
      namespace: monitoring
```

`serviceMonitor.namespace` defaults to the Helm release namespace. Use
`additionalLabels` when the Prometheus installation selects ServiceMonitors by
label.

For secure metrics, the ServiceMonitor uses the Prometheus Pod's projected
ServiceAccount token by default. When `prometheusServiceAccount.name` and
`prometheusServiceAccount.namespace` are both set and `rbac.create=true`, the
chart creates a ClusterRoleBinding granting that ServiceAccount `GET /metrics`.
Leave both fields empty when equivalent metrics-reader RBAC is managed
externally.

The default secure scrape skips certificate verification because
controller-runtime serves the metrics endpoint with its runtime TLS
certificate unless a trusted certificate is configured separately.

If `networkPolicy.enabled=true`, make sure the Prometheus namespace matches the
configured metrics ingress policy.

## CRDs and upgrades

The chart includes generated CRDs under `crds/`. Helm installs them before the
controller resources but does not upgrade or delete them. When a release
changes CRD schemas, apply the target CRDs before upgrading:

```bash
kubectl apply --server-side -f config/crd/bases/
```

Review the
[operations guide](https://github.com/pierinho13/github-platform-operator/blob/main/docs/operations.md)
before upgrading CRD schemas or enabling destructive deletion policies.

## Documentation and support

- [Project source](https://github.com/pierinho13/github-platform-operator)
- [Getting started](https://github.com/pierinho13/github-platform-operator/blob/main/docs/getting-started.md)
- [Custom resources](https://github.com/pierinho13/github-platform-operator/blob/main/docs/resources.md)
- [Operations](https://github.com/pierinho13/github-platform-operator/blob/main/docs/operations.md)
- [Releases](https://github.com/pierinho13/github-platform-operator/releases)
- [Issues and support](https://github.com/pierinho13/github-platform-operator/issues)

Licensed under the Apache License 2.0.
