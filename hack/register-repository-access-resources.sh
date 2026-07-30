#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f PROJECT || ! -d config/crd || ! -d config/samples ]]; then
  echo "Run this script from the github-platform-operator repository root." >&2
  exit 1
fi

if [[ ! -x bin/kustomize ]]; then
  make kustomize >/dev/null
fi

add_kustomize_resource() {
  local directory="$1"
  local resource="$2"

  if grep -Fq -- "${resource}" "${directory}/kustomization.yaml"; then
    return
  fi

  (
    cd "${directory}"
    ../../bin/kustomize edit add resource --no-verify "${resource}"
  )
}

add_kustomize_resource config/crd \
  bases/github.k8sready.com_githubrepositoryteamaccesses.yaml
add_kustomize_resource config/crd \
  bases/github.k8sready.com_githubrepositorycollaborators.yaml
add_kustomize_resource config/samples \
  github_v1alpha1_githubrepositoryteamaccess.yaml
add_kustomize_resource config/samples \
  github_v1alpha1_githubrepositorycollaborator.yaml

python3 - <<'PY'
from pathlib import Path

project = Path("PROJECT")
text = project.read_text(encoding="utf-8")

entries = [
    (
        "GitHubRepositoryTeamAccess",
        """- api:
    crdVersion: v1
    namespaced: true
  controller: true
  domain: k8sready.com
  group: github
  kind: GitHubRepositoryTeamAccess
  path: github.com/pierinho13/github-platform-operator/api/v1alpha1
  version: v1alpha1
""",
    ),
    (
        "GitHubRepositoryCollaborator",
        """- api:
    crdVersion: v1
    namespaced: true
  controller: true
  domain: k8sready.com
  group: github
  kind: GitHubRepositoryCollaborator
  path: github.com/pierinho13/github-platform-operator/api/v1alpha1
  version: v1alpha1
""",
    ),
]

missing = [entry for kind, entry in entries if f"kind: {kind}\n" not in text]
if not missing:
    raise SystemExit(0)

lines = text.splitlines(keepends=True)
resources_index = next(
    (index for index, line in enumerate(lines) if line.rstrip("\n") == "resources:"),
    None,
)
if resources_index is None:
    raise SystemExit("PROJECT does not contain a resources section")

insert_index = len(lines)
for index in range(resources_index + 1, len(lines)):
    line = lines[index]
    if line and not line[0].isspace() and not line.startswith("-"):
        insert_index = index
        break

lines[insert_index:insert_index] = missing
project.write_text("".join(lines), encoding="utf-8")
PY

echo "Repository access CRDs registered in PROJECT and Kustomize."
