#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_DIR="${ROOT_DIR}/config/crd/bases"
DESTINATION_DIR="${ROOT_DIR}/charts/github-platform-operator/crds"

if [[ ! -d "${SOURCE_DIR}" ]]; then
  echo "CRD source directory does not exist: ${SOURCE_DIR}" >&2
  exit 1
fi

mkdir -p "${DESTINATION_DIR}"
find "${DESTINATION_DIR}" -type f -name '*.yaml' -delete

set -- "${SOURCE_DIR}"/*.yaml
if [[ ! -e "$1" ]]; then
  echo "No generated CRDs found under ${SOURCE_DIR}" >&2
  exit 1
fi

cp "${SOURCE_DIR}"/*.yaml "${DESTINATION_DIR}/"

crd_count="$(find "${DESTINATION_DIR}" -type f -name '*.yaml' | wc -l | tr -d ' ')"
echo "Copied ${crd_count} CRDs into the Helm chart"
