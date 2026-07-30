# Operations

## Observe reconciliation

List all managed resources:

```bash
kubectl get \
  ghprovider,ghrepo,ghteamaccess,ghcollab,ghenv,ghsecret,ghvar \
  -A
```

Inspect a resource and its conditions:

```bash
kubectl get ghrepo example-repository -o yaml
```

Common `Ready` condition reasons include:

```text
RepositoryCreated
RepositoryUpdated
RepositoryAvailable
AccessConfigured
InvitationPending
EnvironmentCreated
SecretCreated
SecretUpdated
VariableCreated
VariableUpdated
DependencyUnavailable
ReconciliationFailed
```

View controller logs:

```bash
kubectl logs -f \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system
```

## Deletion policies

The default policy is non-destructive.

| Resource | Safe default | Destructive option |
|---|---|---|
| `GitHubRepository` | `Orphan` | `Delete` |
| `GitHubRepositoryTeamAccess` | `Orphan` | `Revoke` |
| `GitHubRepositoryCollaborator` | `Orphan` | `Revoke` |
| `GitHubEnvironment` | `Orphan` | `Delete` |
| `GitHubActionsSecret` | `Orphan` | `Revoke` |
| `GitHubActionsVariable` | `Orphan` | `Revoke` |

Review the manifest before changing a deletion policy. Kubernetes finalizers
keep the custom resource present until the requested GitHub cleanup succeeds.

## Secret and variable rotation

Update the referenced Kubernetes Secret:

```bash
kubectl patch secret payments-actions-values \
  --namespace default \
  --type merge \
  -p '{"stringData":{"region":"eu-central-1"}}'
```

The secret watch enqueues only the Actions resources that reference the changed
Secret. The status records the source Secret UID and resource version without
recording its value.

## Helm configuration

Show all values:

```bash
helm show values \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0
```

Example override:

```yaml
replicaCount: 2

resources:
  requests:
    cpu: 50m
    memory: 96Mi
  limits:
    cpu: 500m
    memory: 256Mi

metrics:
  enabled: true
  secure: true
```

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0 \
  --namespace github-platform-operator-system \
  --create-namespace \
  --values values.yaml
```

## Upgrade the operator

Helm installs files under the chart's `crds/` directory only during the first
installation. Helm does not upgrade or delete those CRDs automatically.

For a release that changes CRD schemas:

1. Download or check out the target release.
2. Apply the target CRDs.
3. Upgrade the chart.

```bash
kubectl apply -f config/crd/bases/

helm upgrade github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version <target-version> \
  --namespace github-platform-operator-system
```

Review release notes before upgrading between `v1alpha1` releases.

## Uninstall

```bash
helm uninstall github-platform-operator \
  --namespace github-platform-operator-system
```

The operator Deployment and RBAC are removed. Helm intentionally leaves CRDs
and existing custom resources in the cluster.

Before deleting CRDs, inspect all finalizers and remote deletion policies:

```bash
kubectl get \
  ghrepo,ghteamaccess,ghcollab,ghenv,ghsecret,ghvar \
  -A
```

## Troubleshooting

### Resource remains without `READY`

Confirm that the controller is running:

```bash
kubectl get pods -n github-platform-operator-system
kubectl logs \
  deployment/github-platform-operator \
  -n github-platform-operator-system
```

### `DependencyUnavailable`

Check referenced resources and namespaces:

```bash
kubectl get ghprovider
kubectl get ghrepo,ghenv -A
kubectl get secret -A
```

### GitHub returns `403`

The token does not have enough permission, the organization policy blocks the
operation, or the selected GitHub plan does not support that feature.

### Collaborator remains pending

The invited GitHub user must accept the repository invitation. The controller
will observe the accepted access during a later reconciliation.
