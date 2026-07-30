# Releasing GitHub Platform Operator

A release publishes:

1. a GitHub Release created by GoReleaser
2. Linux manager archives for `amd64` and `arm64`
3. a multi-platform controller image at
   `ghcr.io/pierinho13/github-platform-operator`
4. an OCI Helm chart at
   `oci://ghcr.io/pierinho13/charts/github-platform-operator`

## Pre-release checks

Run:

```bash
make generate manifests
make fmt
make vet
make test
make lint
git diff --check
```

Package and validate the chart:

```bash
./hack/package-helm-chart.sh 0.1.0 v0.1.0
```

Validate GoReleaser:

```bash
goreleaser check
```

Confirm the working tree contains only the intended release changes.

## Create a release

The preferred method is:

```text
GitHub → Actions → Publish release → Run workflow
```

Provide an explicit semantic version:

```text
v0.1.0
```

The workflow creates the annotated tag and publishes all artifacts.

A release can also start from an existing tag:

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

Do not reuse or move an existing release tag.

## Verify published artifacts

Check the GitHub Release and image:

```bash
docker pull ghcr.io/pierinho13/github-platform-operator:v0.1.0
```

Inspect the chart:

```bash
helm show chart \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0
```

Install it in a disposable cluster:

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0 \
  --namespace github-platform-operator-system \
  --create-namespace
```

```bash
kubectl rollout status \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system

kubectl get crd | grep github.k8sready.com
```

## First publication in GHCR

Packages may be private after their first publication. Open the package settings
for both the controller image and Helm chart and make them public so users can
install without registry authentication.

Also verify that both packages are connected to this repository and inherit the
intended GitHub Actions permissions.

## Release notes

Release notes should include:

- user-visible features and fixes
- API or CRD changes
- required GitHub permissions
- upgrade steps
- known limitations
- any destructive behavior changes

Because the API is `v1alpha1`, call out incompatible schema or behavior changes
prominently.
