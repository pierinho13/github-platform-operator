# Getting started

## Prerequisites

- Kubernetes 1.30 or newer
- Helm 3 with OCI registry support
- `kubectl`
- a GitHub token with access to the target organization and the resources that
  the operator will manage

Use a disposable repository or organization while evaluating destructive
deletion policies.

## Install the operator

```bash
helm upgrade --install github-platform-operator \
  oci://ghcr.io/pierinho13/charts/github-platform-operator \
  --version 0.1.0 \
  --namespace github-platform-operator-system \
  --create-namespace
```

Check the installation:

```bash
kubectl rollout status \
  deployment/github-platform-operator \
  --namespace github-platform-operator-system

kubectl get crd | grep github.k8sready.com
```

The Helm chart installs these resources:

```text
githubproviderconfigs.github.k8sready.com
githubrepositories.github.k8sready.com
githubrepositoryteamaccesses.github.k8sready.com
githubrepositorycollaborators.github.k8sready.com
githubenvironments.github.k8sready.com
githubactionssecrets.github.k8sready.com
githubactionsvariables.github.k8sready.com
```

## Configure GitHub credentials

Create a Kubernetes Secret containing the GitHub token:

```bash
kubectl create secret generic github-credentials \
  --namespace default \
  --from-literal=token="${GITHUB_TOKEN}"
```

Create a cluster-scoped `GitHubProviderConfig`:

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

```bash
kubectl apply -f provider.yaml
kubectl get ghprovider default
```

For GitHub Enterprise Server, set `spec.apiURL` to the REST API base URL, for
example:

```yaml
apiURL: https://github.example.com/api/v3
```

## Create or adopt a repository

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
  deletionPolicy: Orphan
```

```bash
kubectl apply -f repository.yaml
kubectl wait \
  --for=condition=Ready \
  ghrepo/example-repository \
  --timeout=90s
```

If the repository already exists in the provider organization, the operator
adopts it. Optional fields that are omitted remain unmanaged and retain their
existing GitHub values.

Inspect the observed state:

```bash
kubectl get ghrepo example-repository
kubectl get ghrepo example-repository -o yaml
```

## Next steps

- Add [team or collaborator access](resources.md#repository-access).
- Create an [environment](resources.md#githubenvironment).
- Synchronize [Actions secrets and variables](resources.md#github-actions-secrets-and-variables).
- Review [deletion policies and upgrades](operations.md).
