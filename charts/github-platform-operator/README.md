# GitHub Platform Operator Helm chart

OCI location:

```text
oci://ghcr.io/pierinho13/charts/github-platform-operator
```

## Install

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0 \
  --namespace github-platform-operator-system \
  --create-namespace
```

The controller image defaults to:

```text
ghcr.io/pierinho13/github-platform-operator:<chart appVersion>
```

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

networkPolicy:
  enabled: false

rbac:
  create: true

resources:
  requests:
    cpu: 10m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 128Mi
```

Show every value:

```bash
helm show values \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0
```

## Custom image

```bash
helm upgrade --install github-platform-operator \
  ./charts/github-platform-operator \
  --namespace github-platform-operator-system \
  --create-namespace \
  --set image.repository=example.com/github-platform-operator \
  --set image.tag=dev
```

## CRDs and upgrades

The packaged chart includes generated CRDs under `crds/`, including repository
rulesets, organization members, teams and team memberships. Helm installs them
before the controller resources.

When `rbac.create=true`, the chart also creates the cluster-scoped permissions
needed by every controller, including `GitHubOrganizationMember`, `GitHubTeam`
and `GitHubTeamMembership`.

Helm does not upgrade or delete resources under `crds/`. When a release changes
CRD schemas, apply the target CRDs before running `helm upgrade`:

```bash
kubectl apply -f config/crd/bases/
```

See [operations](../../docs/operations.md) for the complete upgrade and
uninstallation procedure.
