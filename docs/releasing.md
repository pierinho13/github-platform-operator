# Releasing GitHub Platform Operator

Releases publish three artifacts:

1. A GitHub Release created by GoReleaser.
2. A multi-platform controller image at `ghcr.io/pierinho13/github-platform-operator`.
3. An OCI Helm chart at `oci://ghcr.io/pierinho13/charts/github-platform-operator`.

## Create a release

Open **Actions → Publish release → Run workflow** and provide an explicit
semantic version such as:

```text
v0.1.0
```

The workflow creates the tag and publishes all release artifacts.

A release can also be started by pushing an existing semantic tag:

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

## Local chart validation

```bash
make generate manifests
./hack/package-helm-chart.sh 0.1.0 v0.1.0
```

The packaged chart is written to:

```text
dist/helm/github-platform-operator-0.1.0.tgz
```

Validate an installation locally:

```bash
helm template github-platform-operator \
  dist/helm/github-platform-operator-0.1.0.tgz \
  --namespace github-platform-operator-system
```

## Install a published release

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0 \
  --namespace github-platform-operator-system \
  --create-namespace
```

## First publication in GHCR

GitHub Container Registry packages are private by default on first
publication. After publishing the first release, open the package settings for
both the controller image and Helm chart and change their visibility to
**Public** so users can install without authenticating.
