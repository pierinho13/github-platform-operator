# GitHub Platform Operator

[![CI](https://github.com/pierinho13/github-platform-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/pierinho13/github-platform-operator/actions/workflows/ci.yaml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/pierinho13/github-platform-operator)](https://github.com/pierinho13/github-platform-operator)
[![GitHub Release](https://img.shields.io/github/v/release/pierinho13/github-platform-operator?display_name=tag&sort=semver)](https://github.com/pierinho13/github-platform-operator/releases)
[![License](https://img.shields.io/github/license/pierinho13/github-platform-operator)](LICENSE)
[![Kubernetes Operator](https://img.shields.io/badge/Kubernetes-Operator-326CE5?logo=kubernetes&logoColor=white)](docs/getting-started.md)
[![Helm OCI](https://img.shields.io/badge/Helm-OCI-0F1689?logo=helm&logoColor=white)](charts/github-platform-operator/README.md)

A Kubernetes operator for declaratively creating, adopting and continuously
reconciling GitHub repositories and their common platform resources.

GitHub Platform Operator lets platform teams manage GitHub through Kubernetes
custom resources and GitOps workflows instead of manual configuration or
one-off automation scripts.

## Features

- create repositories from scratch or templates, or safely adopt existing ones
- manage repository metadata, visibility, features, merge options and alerts
- protect branches, tags and pushes with repository rulesets
- manage organization members, teams and team memberships
- grant team and direct collaborator access to repositories
- manage GitHub environments
- synchronize Actions secrets and variables
- authenticate with personal access tokens or GitHub Apps
- suspend providers without deleting managed resources
- continuously detect and reconcile configuration drift
- handle shared GitHub API rate limits across controllers
- use explicit, non-destructive deletion policies
- install and upgrade through an OCI Helm chart published in GHCR

> [!NOTE]
> The API currently uses `v1alpha1`. It is suitable for controlled production
> use, compatibility is preserved whenever possible, and any required migration
> steps are documented in the release notes.

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

See [Custom resources](docs/resources.md) for complete schemas, supported
values, examples and reconciliation behavior.

## Install with Helm

The chart is published as an OCI artifact at:

```text
oci://ghcr.io/pierinho13/charts/github-platform-operator
```

Install version `0.4.1`:

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.4.1 \
  --namespace github-platform-operator-system \
  --create-namespace
```

Verify the installation:

```bash
kubectl rollout status \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system

kubectl get crd | grep github.k8sready.com
```

See the [Helm chart documentation](charts/github-platform-operator/README.md)
for values, custom images and CRD upgrade behavior.

## Quick start

Create a Kubernetes Secret containing a GitHub token:

```bash
kubectl create secret generic github-credentials \
  --namespace default \
  --from-literal=token="${GITHUB_TOKEN}"
```

Create a provider:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubProviderConfig
metadata:
  name: default
spec:
  organization: k8sready
  apiURL: https://api.github.com
  credentials:
    secretRef:
      namespace: default
      name: github-credentials
      key: token
```

Create or adopt a repository:

```yaml
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
  topics:
    - kubernetes
    - platform-engineering
  deletionPolicy: Orphan
```

Apply and inspect the resources:

```bash
kubectl apply -f provider.yaml
kubectl apply -f repository.yaml

kubectl wait \
  --for=condition=Ready \
  ghrepo/example-repository \
  --timeout=90s

kubectl get ghprovider
kubectl get ghrepo -A
```

If the repository already exists in the provider organization, the operator
adopts it. Optional fields that are omitted remain unmanaged and retain their
existing GitHub values.

Continue with the [Getting started guide](docs/getting-started.md) to configure
GitHub App authentication, organization teams, repository rulesets and other
managed resources.

## Safe by default

`Orphan` is the default deletion policy. Deleting a Kubernetes custom resource
therefore keeps the corresponding GitHub resource unless an explicit remote
action such as `Archive`, `Delete` or `Revoke` is configured.

Review [Operations](docs/operations.md) before enabling destructive deletion
policies or upgrading CRD schemas.

## Documentation

- [Getting started](docs/getting-started.md)
- [Custom resources and examples](docs/resources.md)
- [Operations, upgrades and troubleshooting](docs/operations.md)
- [Helm chart](charts/github-platform-operator/README.md)
- [Development guide](docs/development.md)
- [Release process](docs/releasing.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

Sample manifests are available under [`config/samples`](config/samples).

## Scope

The project deliberately focuses on common repository-platform workflows
instead of exposing every GitHub API endpoint.

GitHub Actions runners, webhooks, arbitrary repository files, Dependabot
secrets, organization policies, billing and complete enterprise administration
remain outside the current scope.

## Contributing

Issues, feature proposals and pull requests are welcome. Review the
[contributing guidelines](CONTRIBUTING.md) and
[development guide](docs/development.md) before submitting a change.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
