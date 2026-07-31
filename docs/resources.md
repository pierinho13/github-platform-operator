# Custom resources

All resources use:

```text
apiVersion: github.k8sready.com/v1alpha1
```

`GitHubProviderConfig` is cluster-scoped. The remaining resources are
namespaced. References to repositories, teams, environments and Kubernetes
Secrets resolve within the same namespace unless the field explicitly contains
a namespace.

## Short names

| Kind | Short name |
|---|---|
| `GitHubProviderConfig` | `ghprovider` |
| `GitHubOrganizationMember` | `ghorgmember` |
| `GitHubTeam` | `ghteam` |
| `GitHubTeamMembership` | `ghteammember` |
| `GitHubRepository` | `ghrepo` |
| `GitHubRepositoryRuleset` | `ghruleset` |
| `GitHubRepositoryTeamAccess` | `ghteamaccess` |
| `GitHubRepositoryCollaborator` | `ghcollab` |
| `GitHubEnvironment` | `ghenv` |
| `GitHubActionsSecret` | `ghsecret` |
| `GitHubActionsVariable` | `ghvar` |

## `GitHubProviderConfig`

Defines a reusable GitHub organization and authentication configuration.
Exactly one of `credentials.secretRef` and `credentials.githubApp` must be set.

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

GitHub App authentication uses the following shape:

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

The private-key Secret must contain a PEM-encoded PKCS#1 or PKCS#8 RSA key.
Installation tokens are cached in memory and refreshed before expiration.

Set `spec.suspended: true` to stop all remote reconciliation through a provider.
While suspended, controllers do not read credentials or call GitHub and managed
resources report `Ready=False` with reason `ReconciliationSuspended`.
Kubernetes resources and finalizers remain present until the provider is
resumed, except that resources using an `Orphan` deletion policy can still
remove their finalizer without remote cleanup.

The provider cannot be deleted while it is referenced by managed repositories,
repository rulesets, organization members, teams or organization-scoped Actions
resources.

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
  autoInit: true
  deleteBranchOnMerge: true
  vulnerabilityAlerts: true
  isTemplate: false
  mergeOptions:
    allowAutoMerge: true
    allowMergeCommit: false
    allowRebaseMerge: false
    allowSquashMerge: true
    squashMergeCommitTitle: COMMIT_OR_PR_TITLE
    squashMergeCommitMessage: COMMIT_MESSAGES
  deletionPolicy: Orphan
```

Optional settings follow safe adoption semantics:

- omitted field: observe it but do not manage it
- explicitly configured field: reconcile it
- `description: ""` or `homepage: ""`: clear it
- `topics: []`: remove all topics
- `visibility: internal`: require GitHub Enterprise
- `autoInit` and `template`: creation-only and mutually exclusive

Create a repository from a GitHub template with:

```yaml
spec:
  providerConfigRef: default
  name: payments-worker
  visibility: private
  template:
    owner: k8sready
    repository: service-template
    includeAllBranches: false
  deletionPolicy: Orphan
```

The supported merge format values are:

| Field | Values |
|---|---|
| `mergeCommitTitle` | `PR_TITLE`, `MERGE_MESSAGE` |
| `mergeCommitMessage` | `PR_BODY`, `PR_TITLE`, `BLANK` |
| `squashMergeCommitTitle` | `PR_TITLE`, `COMMIT_OR_PR_TITLE` |
| `squashMergeCommitMessage` | `PR_BODY`, `COMMIT_MESSAGES`, `BLANK` |

Repository deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the GitHub repository |
| `Archive` | Archive the GitHub repository |
| `Delete` | Permanently delete the GitHub repository |

`Orphan` is the default.

## `GitHubRepositoryRuleset`

Creates, adopts and continuously reconciles a repository-owned GitHub ruleset.

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubRepositoryRuleset
metadata:
  name: payments-api-protect-main
  namespace: default
spec:
  repositoryRef:
    name: payments-api
  name: protect-main
  target: branch
  enforcement: active
  bypassActors:
    - actorType: Team
      teamSlug: platform
      bypassMode: always
    - actorType: User
      username: release-bot
      bypassMode: pull_request
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
          - squash
  deletionPolicy: Orphan
```

Supported targets:

```text
branch
tag
push
```

Supported enforcement modes:

```text
disabled
active
evaluate
```

Supported rule types:

```text
creation
update
deletion
required_linear_history
merge_queue
required_deployments
required_signatures
pull_request
required_status_checks
non_fast_forward
commit_message_pattern
commit_author_email_pattern
committer_email_pattern
branch_name_pattern
tag_name_pattern
workflows
code_scanning
copilot_code_review
license_compliance_scanning
file_path_restriction
max_file_path_length
file_extension_restriction
max_file_size
```

Rule-specific `parameters` are preserved as schemaless JSON and sent to GitHub.
Parameterless rules omit `parameters`.

Bypass actors can use human-readable identifiers for teams and users:

```yaml
bypassActors:
  - actorType: Team
    teamSlug: platform
    bypassMode: always
  - actorType: User
    username: release-bot
    bypassMode: pull_request
```

The controller resolves `teamSlug` and `username` to GitHub's numeric actor IDs
before creating or updating the ruleset. Existing manifests using `actorID`
remain supported. Use exactly one of `teamSlug` or `actorID` for `Team`, and
exactly one of `username` or `actorID` for `User`. `Integration` and
`RepositoryRole` continue to require `actorID`; `OrganizationAdmin` and
`DeployKey` accept no identifier. Resolving a team slug requires GitHub
organization `Members: read` permission; classic personal access tokens need
`read:org`.

Ruleset management semantics:

- `rules` is the complete desired rule list.
- Omitted `conditions` are left unmanaged.
- Omitted `bypassActors` are left unmanaged.
- `bypassActors: []` removes every managed bypass actor.
- Empty ref exclusions are sent as `exclude: []`, not `null`.
- Rule, condition and bypass-actor ordering does not create false drift.

The controller first looks up the ruleset by `status.rulesetID`. If it is not
available, it adopts a unique repository-owned ruleset with the same name.
Multiple repository-owned rulesets with the same name cause reconciliation to
stop rather than selecting one ambiguously.

Ruleset deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the GitHub ruleset |
| `Delete` | Delete the GitHub ruleset |

`Orphan` is the default. GitHub may reject ruleset operations with `403` when
the selected repository or organization plan does not support the feature.

## Organization membership and teams

These resources require GitHub organization `Members: write` permission.
Classic personal access tokens need an organization scope that permits member
and team administration.

### `GitHubOrganizationMember`

Creates or updates direct organization membership.

```yaml
apiVersion: github.k8sready.com/v1alpha1
kind: GitHubOrganizationMember
metadata:
  name: octocat
  namespace: default
spec:
  providerConfigRef: default
  username: octocat
  role: member
  deletionPolicy: Orphan
```

Supported organization roles are `member` and `admin`. GitHub may report a new
member as `InvitationPending` until the user accepts the organization
invitation.

Membership deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the organization membership |
| `Revoke` | Remove the user from the organization |

### `GitHubTeam`

Creates a team or adopts an existing team with the same name.

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

Supported privacy values are `closed` and `secret`. When privacy is omitted, new
teams use `closed`; adopted teams keep their current value. A team configured
with `Delete` remains protected while a managed `GitHubTeamMembership`
references it.

Team deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the GitHub team |
| `Delete` | Delete the GitHub team |

### `GitHubTeamMembership`

Assigns a GitHub user to a managed team.

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
  role: maintainer
  deletionPolicy: Orphan
```

Supported team roles are `member` and `maintainer`. Membership can remain in
`InvitationPending` while the user has not accepted the organization
invitation.

Team membership deletion policies:

| Policy | Result |
|---|---|
| `Orphan` | Keep the team membership |
| `Revoke` | Remove the user from the team |

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
