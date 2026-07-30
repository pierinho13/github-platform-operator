#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_DIR="${ROOT_DIR}/charts/github-platform-operator"
OUTPUT_DIR="${ROOT_DIR}/dist/helm"

CHART_VERSION="${1:-0.1.0}"
APP_VERSION="${2:-v${CHART_VERSION}}"

if [[ "${CHART_VERSION}" == v* ]]; then
  echo "The Helm chart version must not include the v prefix: ${CHART_VERSION}" >&2
  exit 1
fi

if ! command -v helm >/dev/null 2>&1; then
  echo "helm is required to package the chart" >&2
  exit 1
fi

"${ROOT_DIR}/hack/sync-helm-crds.sh"

mkdir -p "${OUTPUT_DIR}"
rm -f "${OUTPUT_DIR}"/github-platform-operator-*.tgz

helm lint "${CHART_DIR}"

helm template github-platform-operator "${CHART_DIR}" \
  --namespace github-platform-operator-system \
  --set "image.tag=${APP_VERSION}" \
  >/dev/null

helm package "${CHART_DIR}" \
  --destination "${OUTPUT_DIR}" \
  --version "${CHART_VERSION}" \
  --app-version "${APP_VERSION}"
