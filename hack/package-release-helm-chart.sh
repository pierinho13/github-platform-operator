#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 4 ]]; then
  cat >&2 <<'EOF'
Usage:
  hack/package-release-helm-chart.sh <version> [app-version] [current-ref] [previous-ref]

Examples:
  hack/package-release-helm-chart.sh 0.6.0 v0.6.0
  hack/package-release-helm-chart.sh 0.6.1 v0.6.1 HEAD v0.6.0

The version must come from the existing release/version calculation. This
script only prepares release-specific Helm metadata and then delegates the
actual packaging and optional signing to package-helm-chart.sh.
EOF
  exit 2
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_FILE="${ROOT_DIR}/charts/github-platform-operator/Chart.yaml"

CHART_VERSION="${1#v}"
APP_VERSION="${2:-v${CHART_VERSION}}"
CURRENT_REF="${3:-HEAD}"
PREVIOUS_REF="${4:-}"

chart_backup="$(mktemp)"

restore_chart() {
  if [[ -f "${chart_backup}" ]]; then
    cp "${chart_backup}" "${CHART_FILE}"
    rm -f "${chart_backup}"
  fi
}

trap restore_chart EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cp "${CHART_FILE}" "${chart_backup}"

prepare_args=(
  --chart "${CHART_FILE}"
  --repo-root "${ROOT_DIR}"
  --version "${CHART_VERSION}"
  --app-version "${APP_VERSION}"
  --current-ref "${CURRENT_REF}"
)

if [[ -n "${PREVIOUS_REF}" ]]; then
  prepare_args+=(--previous-ref "${PREVIOUS_REF}")
fi

python3 "${ROOT_DIR}/hack/prepare-artifacthub-chart.py" "${prepare_args[@]}"

(
  cd "${ROOT_DIR}"

  # Keep the existing packaging implementation as the single source of truth
  # for CRD/chart validation and Helm provenance signing.
  ./hack/package-helm-chart.sh     "${CHART_VERSION}"     "${APP_VERSION}"
)

chart="${ROOT_DIR}/dist/helm/github-platform-operator-${CHART_VERSION}.tgz"

if [[ ! -f "${chart}" ]]; then
  echo "Helm chart was not generated: ${chart}" >&2
  exit 1
fi

echo "Release Helm metadata prepared automatically:"
echo "  chart version: ${CHART_VERSION}"
echo "  app version:   ${APP_VERSION}"
echo "  chart:         ${chart}"
