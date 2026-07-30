# GitHub Platform Operator Helm chart

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

## Configuration

Common values:

```yaml
replicaCount: 1

image:
  repository: ghcr.io/pierinho13/github-platform-operator
  tag: ""
  pullPolicy: IfNotPresent

metrics:
  enabled: true
  secure: true
  port: 8443

leaderElection:
  enabled: true

resources:
  requests:
    cpu: 10m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 128Mi
```

## CRDs

The packaged chart contains the generated CRDs under `crds/`. Helm installs
these CRDs before rendering the controller resources.

Helm does not upgrade or delete resources stored under a chart's `crds/`
directory. Before upgrading across a release that changes CRD schemas, apply
the CRDs from the target release and then run `helm upgrade`.
