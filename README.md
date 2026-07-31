# GitHub Platform Operator

[![CI](https://github.com/pierinho13/github-platform-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/pierinho13/github-platform-operator/actions/workflows/ci.yaml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/pierinho13/github-platform-operator)](https://github.com/pierinho13/github-platform-operator)
[![GitHub Release](https://img.shields.io/github/v/release/pierinho13/github-platform-operator?display_name=tag\&sort=semver)](https://github.com/pierinho13/github-platform-operator/releases)
[![License](https://img.shields.io/github/license/pierinho13/github-platform-operator)](LICENSE)
[![Kubernetes Operator](https://img.shields.io/badge/Kubernetes-Operator-326CE5?logo=kubernetes\&logoColor=white)](docs/getting-started.md)
[![Helm OCI](https://img.shields.io/badge/Helm-OCI-0F1689?logo=helm\&logoColor=white)](charts/github-platform-operator/README.md)

A Kubernetes operator for declaratively creating, adopting and continuously
reconciling GitHub repositories and their common platform resources.

GitHub Platform Operator allows platform teams to manage GitHub configuration
through Kubernetes custom resources and GitOps workflows instead of manual
configuration or one-off automation scripts.

It intentionally covers the most common repository-platform workflows without
trying to mirror the entire GitHub API.

## Features

* repository creation from scratch or from templates, with safe adoption
* public, private and internal repository visibility
* descriptions, homepages, topics and repository features
* configurable merge strategies
* automatic branch deletion after merge
* vulnerability alerts and template repositories
* repository rulesets for branch, tag and push targets
* human-readable ruleset bypass actors through team slugs and usernames
* organization members, teams and team memberships
* team and direct collaborator repository access
* GitHub environments
* Actions secrets and variables for repositories, environments and organizations
* personal access token and GitHub App installation authentication
* provider suspension
* shared GitHub API rate-limit handling
* continuous drift reconciliation
* Kubernetes-style status conditions
* explicit and non-destructive deletion policies
* Helm-based installation through GHCR

> [!NOTE]
> The API is currently `v1alpha1` and is suitable for production evaluation and
> controlled deployments. Compatibility is preserved whenever possible, and any
> required migration steps or behavior changes are documented in the release notes.

## Managed resources

| Resource                       | Purpose                                              |
| ------------------------------ | ---------------------------------------------------- |
| `GitHubProviderConfig`         | GitHub organization and authentication configuration |
| `GitHubRepository`             | Repository creation, adoption and configuration      |
| `GitHubRepositoryRuleset`      | Branch, tag and push rulesets                        |
| `GitHubOrganizationMember`     | Organization membership and role management          |
| `GitHubTeam`                   | Organization team creation and adoption              |
| `GitHubTeamMembership`         | Team membership and role management                  |
| `GitHubRepositoryTeamAccess`   | Team access to repositories                          |
| `GitHubRepositoryCollaborator` | Direct collaborator access                           |
| `GitHubEnvironment`            | Repository environments                              |
| `GitHubActionsSecret`          | Repository, environment and organization secrets     |
| `GitHubActionsVariable`        | Repository, environment and organization variables   |

All resources use:

```text
github.k8sready.com/v1alpha1
```

`GitHubProviderConfig` is cluster-scoped. The remaining resources are
namespaced.

## Install with Helm

The Helm chart is published as an OCI artifact in GHCR:

```text
oci://ghcr.io/pierinho13/charts/github-platform-operator
```

Install the operator:

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.1 \
  --namespace github-platform-operator-system \
  --create-namespace
```

Verify the controller:

```bash
kubectl rollout status \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system
```

Verify the installed CRDs:

```bash
kubectl get crd | grep github.k8sready.com
```

## Quick start

### 1. Create the GitHub credentials

Create a Kubernetes Secret containing a GitHub token:

```bash
kubectl create secret generic github-credentials \
  --namespace default \
  --from-literal=token="${GITHUB_TOKEN}"
```

Never place the token directly in a custom resource or Helm values file.

### 2. Create a provider

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

Apply it:

```bash
kubectl apply -f provider.yaml
kubectl get ghprovider default
```

A provider can be temporarily suspended without deleting its managed
resources:

```bash
kubectl patch ghprovider default \
  --type merge \
  -p '{"spec":{"suspended":true}}'
```

While suspended, controllers stop making remote GitHub API requests.

### 3. Create or adopt a repository

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
  homepage: https://example.com
  topics:
    - kubernetes
    - platform-engineering
  features:
    issues: true
    projects: false
    wiki: false
    discussions: true
  mergeOptions:
    allowMergeCommit: true
    allowSquashMerge: true
    allowRebaseMerge: true
  deleteBranchOnMerge: true
  vulnerabilityAlerts: true
  deletionPolicy: Orphan
```

Apply and inspect it:

```bash
kubectl apply -f repository.yaml

kubectl wait \
  --for=condition=Ready \
  ghrepo/example-repository \
  --timeout=90s

kubectl get ghrepo example-repository
kubectl get ghrepo example-repository -o yaml
```

If the repository already exists in the provider organization, the operator
adopts it.

Optional fields that are omitted remain unmanaged and keep their existing
GitHub values.

### 4. Protect the default branch

Create the ruleset initially with `enforcement: disabled` so it can be reviewed
before activation:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepositoryRuleset
metadata:
  name: example-repository-default-branch
  namespace: default
spec:
  repositoryRef:
    name: example-repository
  name: default-branch-protection
  target: branch
  enforcement: disabled
  bypassActors:
    - actorType: Team
      teamSlug: platform
      bypassMode: always
    - actorType: User
      username: release-admin
      bypassMode: always
  conditions:
    refName:
      include:
        - "~DEFAULT_BRANCH"
      exclude: []
  rules:
    - type: deletion
    - type: non_fast_forward
    - type: pull_request
      parameters:
        required_approving_review_count: 1
        dismiss_stale_reviews_on_push: true
        require_code_owner_review: true
        require_last_push_approval: false
        required_review_thread_resolution: true
        allowed_merge_methods:
          - merge
          - squash
          - rebase
  deletionPolicy: Orphan
```

Apply and inspect it:

```bash
kubectl apply -f ruleset.yaml

kubectl wait \
  --for=condition=Ready \
  ghruleset/example-repository-default-branch \
  --timeout=90s

kubectl get ghruleset example-repository-default-branch -o yaml
```

Activate it after validation:

```bash
kubectl patch \
  ghruleset/example-repository-default-branch \
  --type merge \
  -p '{"spec":{"enforcement":"active"}}'
```

## Manage organization teams

Create or adopt an existing GitHub team:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubTeam
metadata:
  name: platform
  namespace: default
spec:
  providerConfigRef: default
  name: Platform
  description: Platform engineering team
  privacy: closed
  deletionPolicy: Orphan
```

Add a team member:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubTeamMembership
metadata:
  name: platform-octocat
  namespace: default
spec:
  teamRef:
    name: platform
  username: octocat
  role: member
  deletionPolicy: Orphan
```

Apply both resources:

```bash
kubectl apply -f team.yaml
kubectl apply -f team-membership.yaml
```

## Safe by default

Remote resources are preserved unless destructive behavior is explicitly
requested.

| Resource                       | Safe default | Destructive option    |
| ------------------------------ | ------------ | --------------------- |
| `GitHubRepository`             | `Orphan`     | `Archive` or `Delete` |
| `GitHubRepositoryRuleset`      | `Orphan`     | `Delete`              |
| `GitHubOrganizationMember`     | `Orphan`     | `Revoke`              |
| `GitHubTeam`                   | `Orphan`     | `Delete`              |
| `GitHubTeamMembership`         | `Orphan`     | `Revoke`              |
| `GitHubRepositoryTeamAccess`   | `Orphan`     | `Revoke`              |
| `GitHubRepositoryCollaborator` | `Orphan`     | `Revoke`              |
| `GitHubEnvironment`            | `Orphan`     | `Delete`              |
| `GitHubActionsSecret`          | `Orphan`     | `Revoke`              |
| `GitHubActionsVariable`        | `Orphan`     | `Revoke`              |

Deleting a Kubernetes resource that uses `Orphan` keeps the corresponding
GitHub resource unchanged.

Review deletion policies carefully before using `Archive`, `Delete` or
`Revoke`.

## GitHub authentication

The operator supports:

* personal access tokens stored in Kubernetes Secrets
* GitHub App installation authentication
* custom GitHub API endpoints for GitHub Enterprise Server

GitHub App authentication is recommended for production environments when
short-lived credentials and scoped repository access are required.

Use only the GitHub permissions required by the resources being managed.

## Observe reconciliation

List managed resources:

```bash
kubectl get \
  ghprovider,ghrepo,ghruleset,ghorgmember,ghteam,ghteammember, \
  ghteamaccess,ghcollab,ghenv,ghsecret,ghvar \
  -A
```

View controller logs:

```bash
kubectl logs -f \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system
```

Inspect status conditions:

```bash
kubectl get ghrepo example-repository -o yaml
```

## CRD upgrades

Helm installs CRDs from the chart's `crds/` directory during the first
installation, but Helm does not automatically upgrade or delete them.

When upgrading to a release that changes CRD schemas, apply the target CRDs
before upgrading the chart:

```bash
kubectl apply -f config/crd/bases/

helm upgrade github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version <target-version> \
  --namespace github-platform-operator-system
```

Review the release notes before upgrading between `v1alpha1` releases.

## Documentation

* [Getting started](docs/getting-started.md)
* [Custom resources and examples](docs/resources.md)
* [Operations, upgrades and troubleshooting](docs/operations.md)
* [Development guide](docs/development.md)
* [Release process](docs/releasing.md)
* [Helm chart](charts/github-platform-operator/README.md)
* [Security policy](SECURITY.md)
* [Contributing](CONTRIBUTING.md)

Sample manifests are available under [`config/samples`](config/samples).

## Development

Run the same checks used by CI:

```bash
make generate manifests
make fmt
make vet
make test
make lint
git diff --check
```

Validate the Helm chart:

```bash
helm lint ./charts/github-platform-operator
```

See the [development guide](docs/development.md) for the complete local
controller and end-to-end testing workflow.

## Scope

The project is deliberately smaller than a general-purpose GitHub provider.

It focuses on making repositories and their common organization resources
ready for teams to work and deploy without exposing every GitHub API endpoint.

The following areas currently remain outside its scope:

* GitHub Actions runners
* webhooks
* arbitrary repository files
* Dependabot secrets
* organization policies
* complete organization administration
* billing and enterprise administration

## Contributing

Issues, feature proposals and pull requests are welcome.

Before submitting a change, review:

* [Contributing guidelines](CONTRIBUTING.md)
* [Development guide](docs/development.md)
* [Security policy](SECURITY.md)

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
