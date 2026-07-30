# Contributing

Thank you for contributing to `github-platform-operator`.

The project aims to remain small, useful and easy to operate. New features
should solve a common repository-platform workflow without turning the project
into a complete GitHub API provider.

## Start here

- [Development guide](docs/development.md)
- [Custom resources](docs/resources.md)
- [Security policy](SECURITY.md)

Clone the repository:

```bash
git clone https://github.com/pierinho13/github-platform-operator.git
cd github-platform-operator
```

Build the manager:

```bash
go build -o bin/manager ./cmd
```

## Branches and commits

Create a focused branch from `main`:

```bash
git switch main
git pull --ff-only
git switch -c feat/my-change
```

Use clear commit messages, for example:

```text
feat(actions): add environment variables
fix(repository): preserve unmanaged topics
docs: update Helm installation
test(access): cover pending invitations
```

## Pull request checklist

Before opening a pull request:

```bash
make generate manifests
make fmt
make vet
make test
make lint
git diff --check
```

A pull request should:

- explain the problem and the chosen scope
- include tests for changed behavior
- include generated files when API or RBAC markers change
- update user documentation and samples when behavior changes
- avoid unrelated refactors
- preserve safe adoption and non-destructive defaults

## Helm and release validation

Validate the chart locally:

```bash
./hack/package-helm-chart.sh 0.1.0-dev v0.1.0-dev
```

Validate GoReleaser after packaging the chart:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

The release workflow and maintainer steps are documented in
[`docs/releasing.md`](docs/releasing.md).

## Bug reports

Include:

- operator version
- Kubernetes version
- installation method
- affected custom resource
- relevant status conditions and controller logs
- reproduction steps

Never include real GitHub tokens, Actions secret values, kubeconfig contents or
other credentials.

## Feature requests

Describe:

- the platform problem
- the expected declarative API
- why it belongs in this focused operator instead of a general provider
- the smallest useful implementation

## Security issues

Do not report vulnerabilities in public issues. Follow
[`SECURITY.md`](SECURITY.md).
