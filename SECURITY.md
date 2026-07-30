# Security Policy

## Supported versions

Security fixes are applied to the latest released version of
`github-platform-operator`.

## Reporting a vulnerability

Do not report security vulnerabilities through public GitHub issues.

Use GitHub's private vulnerability reporting feature for this repository when
available. Include:

- a clear description
- affected versions
- reproduction steps
- potential impact
- suggested mitigation, when known

Do not include real tokens, Secret values, kubeconfig files, private repository
contents or other sensitive data.

## Security model

The operator needs access to two categories of Kubernetes Secrets:

1. GitHub credentials referenced by `GitHubProviderConfig`.
2. Values referenced by `GitHubActionsSecret` and `GitHubActionsVariable`.

The current controller watches Secrets so that Actions values rotate
automatically. Its cluster role therefore includes `get`, `list` and `watch`
for Kubernetes Secrets.

Treat installation of the operator as a privileged cluster operation:

- run it in a dedicated namespace
- restrict who can create provider and Actions resources
- limit access to the controller service account
- enable Kubernetes Secret encryption at rest
- prefer an external secret manager for production values
- audit changes to deletion policies and provider references

## GitHub credentials

Use the minimum GitHub permissions required for the resources being managed.
Prefer short-lived GitHub App installation tokens when practical. Rotate
long-lived personal access tokens regularly.

Never place the GitHub token directly in a custom resource or Helm values.
Reference it through a Kubernetes Secret.

## Actions secrets and variables

`GitHubActionsSecret` values are read from Kubernetes, encrypted with GitHub's
public key and sent to GitHub. The plaintext value is not stored in custom
resource status.

Avoid logging or debugging changes that could print source Secret data.

`GitHubActionsVariable` also reads from a Kubernetes Secret for a consistent
API, but the resulting GitHub variable is not confidential. Never use
`GitHubActionsVariable` for passwords, tokens or private keys.

## Destructive operations

Remote deletion and access revocation require explicit policies:

```text
Delete
Revoke
```

The default is `Orphan`.

Restrict permission to update or delete the custom resources because a user who
can change the deletion policy may trigger destructive GitHub operations.

## Public disclosure

After a fix is available, maintainers may publish a security advisory describing
affected versions, impact and remediation.
