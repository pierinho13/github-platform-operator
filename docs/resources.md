# Custom resources

All resources use:

```text
apiVersion: github.k8sready.com/v1alpha1
```

`GitHubProviderConfig` is cluster-scoped. The remaining resources are
namespaced. References to repositories, environments and Kubernetes Secrets
resolve within the same namespace unless the field explicitly contains a
namespace.

## Short names

| Kind | Short name |
|---|---|
| `GitHubProviderConfig` | `ghprovider` |
| `GitHubRepository` | `ghrepo` |
| `GitHubRepositoryTeamAccess` | `ghteamaccess` |
| `GitHubRepositoryCollaborator` | `ghcollab` |
| `GitHubEnvironment` | `ghenv` |
| `GitHubActionsSecret` | `ghsecret` |
| `GitHubActionsVariable` | `ghvar` |

## `GitHubProviderConfig`

Defines a reusable GitHub organization and token reference.

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

The provider cannot be deleted while it is referenced by managed repositories
or organization-scoped Actions resources.

## `GitHubRepository`

Creates a repository or adopts an existing repository with the same name.

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepository
metadata:
  name: payments-api
  namespace: default
spec:
  providerConfigRef: default
  name: payments-api
  visibility: private
  description: Payments API
  homepage: https://payments.example.com
  topics:
    - golang
    - kubernetes
    - payments
  features:
    issues: true
    projects: false
    wiki: false
    discussions: true
  deletionPolicy: Orphan
```

Optional settings follow safe adoption semantics:

- omitted field: observe it but do not manage it
- explicitly configured field: reconcile it
- `description: ""` or `homepage: ""`: clear it
- `topics: []`: remove all topics

Repository deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the GitHub repository |
| `Delete` | Delete the GitHub repository |

`Orphan` is the default.

## Repository access

### `GitHubRepositoryTeamAccess`

Assigns an existing organization team to a repository.

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepositoryTeamAccess
metadata:
  name: payments-api-platform
  namespace: default
spec:
  repositoryRef:
    name: payments-api
  teamSlug: platform
  permission: maintain
  deletionPolicy: Orphan
```

### `GitHubRepositoryCollaborator`

Grants direct repository access to a GitHub user.

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepositoryCollaborator
metadata:
  name: payments-api-octocat
  namespace: default
spec:
  repositoryRef:
    name: payments-api
  username: octocat
  permission: push
  deletionPolicy: Orphan
```

Supported permissions:

```text
pull
triage
push
maintain
admin
```

A user outside the organization may remain in `InvitationPending` until the
GitHub invitation is accepted.

Access deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the current GitHub access |
| `Revoke` | Remove the team or collaborator access |

## `GitHubEnvironment`

Creates or adopts a basic repository environment.

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubEnvironment
metadata:
  name: payments-api-production
  namespace: default
spec:
  repositoryRef:
    name: payments-api
  name: production
  deletionPolicy: Orphan
```

Environment deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the GitHub environment |
| `Delete` | Delete the GitHub environment |

An environment configured with `Delete` remains protected while a managed
Actions secret or variable still references it.

## GitHub Actions secrets and variables

Both resources read their value from a Kubernetes Secret. Plaintext values are
not accepted in the custom resource.

Create the source Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: payments-actions-values
  namespace: default
type: Opaque
stringData:
  docker-token: replace-me
  region: eu-west-1
```

### Repository secret

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubActionsSecret
metadata:
  name: payments-api-docker-token
  namespace: default
spec:
  target:
    repositoryRef:
      name: payments-api
  name: DOCKER_TOKEN
  valueFrom:
    secretKeyRef:
      name: payments-actions-values
      key: docker-token
  deletionPolicy: Orphan
```

### Environment variable

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubActionsVariable
metadata:
  name: payments-production-region
  namespace: default
spec:
  target:
    environmentRef:
      name: payments-api-production
  name: AWS_REGION
  valueFrom:
    secretKeyRef:
      name: payments-actions-values
      key: region
  deletionPolicy: Orphan
```

### Organization target

Secrets and variables can also target the provider organization:

```yaml
target:
  organization:
    providerConfigRef: default
    visibility: selected
    selectedRepositoryRefs:
      - name: payments-api
```

Supported organization visibility values:

```text
all
private
selected
```

GitHub plan restrictions may limit organization secrets or variables for
private repositories.

Actions resource deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the GitHub secret or variable |
| `Revoke` | Delete it from GitHub |

### Security behavior

For `GitHubActionsSecret`, the operator retrieves GitHub's public key, encrypts
the Kubernetes Secret value and sends only the encrypted value. The plaintext
is not stored in status.

`GitHubActionsVariable` uses the same Kubernetes `secretKeyRef` API for
consistency, but GitHub variables are not confidential and can be read in
plain text by users with sufficient GitHub permissions.

Changes to the referenced Kubernetes Secret trigger reconciliation and rotate
the remote value automatically.
