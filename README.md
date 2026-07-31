# GitHub Platform Operator

[![Go Version](https://img.shields.io/github/go-mod/go-version/pierinho13/github-platform-operator)](https://github.com/pierinho13/github-platform-operator)
[![GitHub Release](https://img.shields.io/github/v/release/pierinho13/github-platform-operator?display_name=tag&sort=semver)](https://github.com/pierinho13/github-platform-operator/releases)
[![License](https://img.shields.io/github/license/pierinho13/github-platform-operator)](LICENSE)

A Kubernetes operator for declaratively creating, adopting and configuring
GitHub repositories and their common platform resources.

It intentionally covers the common platform workflow instead of mirroring the
entire GitHub API:

- repository creation from scratch or templates, with safe adoption
- public, private and internal visibility, metadata, topics and repository features
- merge strategies, automatic branch deletion and vulnerability alerts
- repository rulesets with branch, tag and push targets
- organization members, teams and team memberships
- team and direct collaborator repository access
- GitHub environments
- Actions secrets and variables for repositories, environments and organizations
- personal access token and GitHub App installation authentication
- provider suspension, shared rate-limit handling and drift reconciliation
- Kubernetes-style status and explicit deletion policies

> [!WARNING]
> The API is currently `v1alpha1`. Backward-incompatible changes may occur before
> the first stable release.

## Install with Helm

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0 \
  --namespace github-platform-operator-system \
  --create-namespace
```

Verify the controller and CRDs:

```bash
kubectl get pods -n github-platform-operator-system
kubectl get crd | grep github.k8sready.com
```

## Quick start

Create the GitHub token Secret:

```bash
kubectl create secret generic github-credentials \
  --namespace default \
  --from-literal=token="${GITHUB_TOKEN}"
```

Create a reusable provider:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubProviderConfig
metadata:
  name: default
spec:
  organization: k8sready
  apiURL: https://api.github.com
  suspended: false
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
  features:
    issues: true
    projects: false
    wiki: false
    discussions: true
  deletionPolicy: Orphan
```

Apply and inspect it:

```bash
kubectl apply -f provider.yaml
kubectl apply -f repository.yaml
kubectl get ghprovider
kubectl get ghrepo -A
```

Create a ruleset after the repository is ready:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepositoryRuleset
metadata:
  name: example-repository-protect-main
  namespace: default
spec:
  repositoryRef:
    name: example-repository
  name: protect-main
  target: branch
  enforcement: disabled
  bypassActors:
    - actorType: Team
      teamSlug: platform
      bypassMode: always
  conditions:
    refName:
      include:
        - "~DEFAULT_BRANCH"
      exclude: []
  rules:
    - type: deletion
    - type: non_fast_forward
  deletionPolicy: Orphan
```

```bash
kubectl apply -f ruleset.yaml
kubectl get ghruleset -A
```

`Orphan` is the safe default: deleting the Kubernetes resource keeps the remote
GitHub resource. Repositories can also be archived instead of deleted.
Destructive remote deletion or access revocation must be requested explicitly.

## Documentation

- [Getting started](docs/getting-started.md)
- [Custom resources and examples](docs/resources.md)
- [Operations, upgrades and troubleshooting](docs/operations.md)
- [Development guide](docs/development.md)
- [Release process](docs/releasing.md)
- [Helm chart](charts/github-platform-operator/README.md)
- [Security policy](SECURITY.md)
- [Contributing](CONTRIBUTING.md)

Sample manifests are also available under [`config/samples`](config/samples).

## Scope

The project is deliberately smaller than a general-purpose GitHub provider.
It focuses on making a repository ready for a team to work and deploy without
trying to expose every GitHub API resource.

Features such as runners, webhooks, repository files, Dependabot secrets,
organization policies and complete organization administration remain outside
the current scope.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
