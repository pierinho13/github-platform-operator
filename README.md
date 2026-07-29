# GitHub Platform Operator

[![Go Version](https://img.shields.io/github/go-mod/go-version/pierinho13/github-platform-operator)](https://github.com/pierinho13/github-platform-operator)
[![GitHub Release](https://img.shields.io/github/v/release/pierinho13/github-platform-operator?display_name=tag&sort=semver)](https://github.com/pierinho13/github-platform-operator/releases)
[![License](https://img.shields.io/github/license/pierinho13/github-platform-operator)](LICENSE)

A Kubernetes operator for declaratively provisioning and managing GitHub repositories.

`github-platform-operator` lets platform teams describe a GitHub repository as a Kubernetes custom resource. The controller continuously reconciles the desired state stored in Kubernetes with the actual repository in GitHub.

> [!WARNING]
> This project is currently in **early alpha**. APIs and behavior may change without notice.
>
> Deleting a `GitHubRepository` custom resource currently **permanently deletes the corresponding GitHub repository**.

## Features

The current implementation supports:

- Creating repositories inside a GitHub organization.
- Public and private repository visibility.
- Idempotent reconciliation.
- Updating the visibility of existing repositories.
- Detecting and correcting visibility drift.
- Reporting the observed repository state through Kubernetes status conditions.
- Deleting the GitHub repository when its Kubernetes custom resource is deleted.
- GitHub Enterprise-compatible API base URL configuration through `GITHUB_API_URL`.
- Local development with Kind and `make run`.
- Unit and controller tests using `envtest`.

## Example

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepository
metadata:
  name: example-repository
  namespace: default
spec:
  organization: k8sready
  name: example-repository
  visibility: private
```

Apply it with:

```bash
kubectl apply -f repository.yaml
```

The operator creates:

```text
https://github.com/k8sready/example-repository
```

Check the resource:

```bash
kubectl get githubrepositories
```

Or use the short name:

```bash
kubectl get ghrepo
```

Example output:

```text
NAME                 ORGANIZATION   REPOSITORY          VISIBILITY   READY   AGE
example-repository   k8sready      example-repository  private      True    30s
```

Inspect its complete status:

```bash
kubectl get ghrepo example-repository -o yaml
```

Example status:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: RepositoryAvailable
      message: GitHub repository is synchronized
  observedGeneration: 1
  repositoryId: 123456789
  url: https://github.com/k8sready/example-repository
  visibility: private
```

## How it works

```text
GitHubRepository CR
        |
        v
controller-runtime
        |
        v
GitHubRepositoryReconciler
        |
        v
GitHub REST API
        |
        v
Repository created, updated or deleted
```

The controller watches `GitHubRepository` resources and compares their desired state with GitHub.

During reconciliation it:

1. Reads the custom resource.
2. Adds a finalizer when required.
3. Looks up the repository in GitHub.
4. Creates it when it does not exist.
5. Updates its visibility when drift is detected.
6. Updates the custom resource status.
7. Deletes the remote repository before removing the finalizer when the custom resource is deleted.

## Requirements

- Go
- Docker
- `kubectl`
- A Kubernetes cluster
- Kind for local testing
- A GitHub token allowed to create, update and delete repositories in the target organization

The operator currently reads authentication from:

```text
GITHUB_TOKEN
```

An alternative API endpoint can be configured with:

```text
GITHUB_API_URL
```

The default value is:

```text
https://api.github.com
```

## Local development

### 1. Create a Kind cluster

```bash
kind create cluster --name github-platform-operator
```

Verify the active context:

```bash
kubectl config current-context
```

Expected:

```text
kind-github-platform-operator
```

### 2. Generate code and manifests

```bash
make generate manifests
```

### 3. Install the CRD

```bash
make install
```

Verify it:

```bash
kubectl get crd githubrepositories.github.k8sready.com
```

### 4. Configure GitHub authentication

```bash
export GITHUB_TOKEN='your-token'
```

For GitHub Enterprise Server or a compatible API endpoint:

```bash
export GITHUB_API_URL='https://github.example.com/api/v3'
```

### 5. Run the controller locally

```bash
make run
```

The controller runs as a process on your machine and connects to the Kubernetes cluster selected by your current kubeconfig context.

### 6. Apply a resource

```bash
kubectl apply -f config/samples/github_v1alpha1_githubrepository.yaml
```

Watch the resources:

```bash
kubectl get ghrepo -w
```

## Updating repository visibility

Change the custom resource:

```bash
kubectl patch ghrepo example-repository \
  --type merge \
  -p '{"spec":{"visibility":"public"}}'
```

The operator updates the existing GitHub repository and then reflects the observed state in `.status.visibility`.

## Deleting a repository

> [!CAUTION]
> The following command deletes both the Kubernetes custom resource and the real GitHub repository.

```bash
kubectl delete ghrepo example-repository
```

Deletion uses a Kubernetes finalizer:

```text
Delete custom resource
        |
        v
Kubernetes sets deletionTimestamp
        |
        v
Operator deletes GitHub repository
        |
        v
Operator removes finalizer
        |
        v
Kubernetes removes custom resource
```

If the GitHub repository has already been deleted manually, reconciliation treats the deletion as successful and removes the finalizer.

## Running inside Kubernetes

Build the controller image:

```bash
make docker-build IMG=github-platform-operator:v0.1.0
```

For Kind, load it directly:

```bash
kind load docker-image \
  github-platform-operator:v0.1.0 \
  --name github-platform-operator
```

Deploy the controller:

```bash
make deploy IMG=github-platform-operator:v0.1.0
```

Create the authentication secret:

```bash
kubectl create secret generic github-platform-operator-credentials \
  --namespace github-platform-operator-system \
  --from-literal=GITHUB_TOKEN="${GITHUB_TOKEN}"
```

Inject the secret into the controller Deployment:

```bash
kubectl set env \
  deployment/github-platform-operator-controller-manager \
  --namespace github-platform-operator-system \
  --from=secret/github-platform-operator-credentials
```

Wait for the rollout:

```bash
kubectl rollout status \
  deployment/github-platform-operator-controller-manager \
  --namespace github-platform-operator-system
```

View logs:

```bash
kubectl logs -f \
  deployment/github-platform-operator-controller-manager \
  --namespace github-platform-operator-system \
  --container manager
```

## Testing

Run formatting, static checks and tests:

```bash
make fmt
make vet
make test
```

Run the linter:

```bash
make lint
```

Run the end-to-end suite:

```bash
make test-e2e
```

The generated test setup uses Kind, builds the controller image, installs the CRDs and deploys the controller manager.

## API

### `GitHubRepository`

API group:

```text
github.k8sready.com
```

Version:

```text
v1alpha1
```

Kind:

```text
GitHubRepository
```

### Spec

| Field | Type | Required | Default | Description |
|---|---|---:|---|---|
| `organization` | string | Yes | — | GitHub organization that owns the repository |
| `name` | string | Yes | — | Name of the GitHub repository |
| `visibility` | string | No | `private` | Repository visibility: `public` or `private` |

`organization` and `name` are immutable after creation.

### Status

| Field | Description |
|---|---|
| `repositoryId` | Numeric repository identifier returned by GitHub |
| `url` | GitHub repository URL |
| `visibility` | Visibility currently observed in GitHub |
| `observedGeneration` | Last Kubernetes generation successfully reconciled |
| `conditions` | Kubernetes-style reconciliation conditions |

## Project structure

```text
.
├── api/
│   └── v1alpha1/
│       ├── githubrepository_types.go
│       └── groupversion_info.go
├── cmd/
│   └── main.go
├── config/
│   ├── crd/
│   ├── default/
│   ├── manager/
│   ├── rbac/
│   └── samples/
├── internal/
│   ├── controller/
│   │   └── githubrepository_controller.go
│   └── github/
│       ├── client.go
│       └── rest_client.go
└── test/
```

## Security considerations

- Never commit GitHub tokens to the repository.
- Prefer short-lived GitHub App installation tokens for production.
- Use the minimum permissions needed by the operator.
- Store credentials in a Kubernetes Secret or an external secret manager.
- Restrict who can create or delete `GitHubRepository` resources through Kubernetes RBAC.
- Treat deletion permissions as highly privileged.
- Test destructive behavior only against disposable repositories and organizations.

The current alpha version uses `GITHUB_TOKEN`. GitHub App authentication is planned for a future release.

## Roadmap

Planned capabilities include:

- GitHub App authentication.
- Configurable deletion policies such as `Archive`, `Orphan` and `Delete`.
- Repository descriptions, topics and default branches.
- Repository templates.
- Teams and permissions.
- Rulesets and branch protection.
- GitHub Actions configuration.
- Environments, variables and secrets.
- Reusable repository profiles.
- Drift reporting modes.
- Kubernetes events and Prometheus metrics.
- Helm chart.
- Operator Lifecycle Manager bundle.
- GitHub Enterprise Server validation.
- Admission webhooks and stronger API validation.

## Project status

This project is under active development and should currently be considered experimental.

The API version is `v1alpha1`, which means backward-incompatible changes may occur before the first stable release.

## Contributing

Issues, ideas and pull requests are welcome.

Before submitting a change:

```bash
make generate manifests
make fmt
make vet
make test
make lint
```

When changing API types or Kubebuilder markers, include the regenerated CRDs and generated Go code in the same pull request.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE).
