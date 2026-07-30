# Development

## Prerequisites

- the Go version declared in `go.mod`
- Docker
- `kubectl`
- Kind
- Helm
- GNU Make

## Local controller workflow

Create a local cluster:

```bash
kind create cluster --name github-platform-operator
```

Generate code and manifests:

```bash
make generate manifests
```

Install the CRDs:

```bash
make install
```

Create a PAT or GitHub App Secret and provider using the examples in
[`config/samples`](../config/samples).

Run the controller against the active kubeconfig context:

```bash
make run
```

## Quality checks

Run the same checks expected by CI:

```bash
make generate manifests
make fmt
make vet
make test
make lint
git diff --check
```

Changes to API types or Kubebuilder markers must include regenerated deepcopy
code, CRDs and RBAC. Changes to GitHub request payloads should include contract
tests that assert the serialized JSON sent by the REST client. Every bug fix
should add a regression test that fails without the fix.

## Build and deploy an image

```bash
make docker-build IMG=github-platform-operator:dev
kind load docker-image \
  github-platform-operator:dev \
  --name github-platform-operator
make deploy IMG=github-platform-operator:dev
```

Remove the deployment and CRDs:

```bash
make undeploy
make uninstall
```

## End-to-end tests

```bash
make test-e2e
```

The suite builds the manager image, loads it into Kind, installs the CRDs and
validates the deployed controller and metrics endpoint. A Kind cluster must
exist before running the suite. To reuse the cluster from the local workflow:

```bash
KIND_CLUSTER=github-platform-operator make test-e2e
```

Alternatively, use the default E2E cluster name:

```bash
kind create cluster --name kind
make test-e2e
kind delete cluster --name kind
```

## Validate the Helm chart

```bash
make generate manifests
./hack/package-helm-chart.sh 0.1.0-dev v0.1.0-dev
```

The package is written under:

```text
dist/helm/
```

Render it without installing:

```bash
helm template github-platform-operator \
  dist/helm/github-platform-operator-0.1.0-dev.tgz \
  --namespace github-platform-operator-system
```

## Project layout

```text
.
├── api/v1alpha1/              Custom resource types
├── charts/                    Helm chart
├── cmd/                       Controller manager entrypoint
├── config/                    Kubebuilder CRDs, RBAC and samples
├── docs/                      User and maintainer documentation
├── hack/                      Generation and packaging scripts
├── internal/controller/       Reconcilers
├── internal/github/           GitHub REST client
└── test/                      Controller and end-to-end tests
```
