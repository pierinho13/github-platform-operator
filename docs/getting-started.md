# Getting started

## Prerequisites

- Kubernetes 1.30 or newer
- Helm 3 with OCI registry support
- `kubectl`
- either a GitHub token or an installed GitHub App with access to the target
  organization and the resources that the operator will manage

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
githubrepositoryrulesets.github.k8sready.com
githubrepositoryteamaccesses.github.k8sready.com
githubrepositorycollaborators.github.k8sready.com
githubenvironments.github.k8sready.com
githubactionssecrets.github.k8sready.com
githubactionsvariables.github.k8sready.com
```

## Configure GitHub credentials

A provider must configure exactly one authentication method:

- an existing GitHub token through `credentials.secretRef`
- a GitHub App installation through `credentials.githubApp`

### Personal access token

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

### GitHub App installation

Create a Secret from the PEM private key downloaded for the GitHub App:

```bash
kubectl create secret generic github-app-credentials \
  --namespace github-platform-operator-system \
  --from-file=private-key.pem=/path/to/github-app.private-key.pem
```

Create the provider:

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubProviderConfig
metadata:
  name: github-app
spec:
  organization: k8sready
  apiURL: https://api.github.com
  credentials:
    githubApp:
      appID: Iv1.REPLACE_ME
      installationID: 12345678
      privateKeySecretRef:
        namespace: github-platform-operator-system
        name: github-app-credentials
        key: private-key.pem
```

`appID` accepts the GitHub App client ID or numeric app ID. The operator creates
short-lived installation access tokens and refreshes them before expiration.
PKCS#1 and PKCS#8 RSA private keys are supported.

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

## Protect the repository with a ruleset

Create a disabled ruleset first so its payload and status can be inspected
without enforcing it immediately:

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
kubectl wait \
  --for=condition=Ready \
  ghruleset/example-repository-protect-main \
  --timeout=90s
kubectl get ghruleset example-repository-protect-main -o yaml
```

After validating the observed state, activate it:

```bash
kubectl patch ghruleset example-repository-protect-main \
  --type merge \
  -p '{"spec":{"enforcement":"active"}}'
```

The operator resolves team slugs and usernames to numeric GitHub actor IDs. A
GitHub App or fine-grained token using `teamSlug` needs organization
`Members: read` permission in addition to the permissions required to manage
rulesets. Classic personal access tokens need `read:org`. Existing `actorID` manifests remain valid.

GitHub may return `403` when the repository or organization plan does not
support rulesets. Testing against a disposable repository is recommended.

## Next steps

- Review the complete [`GitHubRepositoryRuleset`](resources.md#githubrepositoryruleset) API.
- Add [team or collaborator access](resources.md#repository-access).
- Create an [environment](resources.md#githubenvironment).
- Synchronize [Actions secrets and variables](resources.md#github-actions-secrets-and-variables).
- Review [deletion policies and upgrades](operations.md).
