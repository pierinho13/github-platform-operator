# GitHub Platform Operator

[![Go Version](https://img.shields.io/github/go-mod/go-version/pierinho13/github-platform-operator)](https://github.com/pierinho13/github-platform-operator)
[![GitHub Release](https://img.shields.io/github/v/release/pierinho13/github-platform-operator?display_name=tag&sort=semver)](https://github.com/pierinho13/github-platform-operator/releases)
[![License](https://img.shields.io/github/license/pierinho13/github-platform-operator)](LICENSE)

A small Kubernetes operator for declaratively creating, adopting and configuring
GitHub repositories.

It intentionally covers the common platform workflow instead of mirroring the
entire GitHub API:

- repository creation and safe adoption
- visibility, description, homepage, topics and repository features
- team and direct collaborator access
- GitHub environments
- Actions secrets and variables for repositories, environments and organizations
- Kubernetes-style status, drift reconciliation and explicit deletion policies

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

`Orphan` is the safe default: deleting the Kubernetes resource keeps the remote
GitHub resource. Destructive remote deletion or access revocation must be
requested explicitly.

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

Features such as runners, webhooks, branch rulesets, repository files,
Dependabot secrets and complete organization administration are outside the
current scope.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
