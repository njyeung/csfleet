#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-"${ROOT}/dist"}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
ARCHIVE_NAME="${ARCHIVE_NAME:-csfleet-${GOOS}-${GOARCH}.tar.gz}"
STAGE_PARENT="${OUT_DIR}/package"
STAGE="${STAGE_PARENT}/csfleet"
ARCHIVE="${OUT_DIR}/${ARCHIVE_NAME}"

mkdir -p "${OUT_DIR}"
rm -rf "${STAGE_PARENT}"
mkdir -p "${STAGE}/frontend"

if [[ "${SKIP_NPM_CI:-0}" != "1" ]]; then
  npm --prefix "${ROOT}/frontend" ci
fi
npm --prefix "${ROOT}/frontend" run build

(
  cd "${ROOT}/orchestration"
  CGO_ENABLED="${CGO_ENABLED:-0}" GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -trimpath -ldflags="-s -w" -o "${STAGE}/csfleet" .
)

cp -R "${ROOT}/frontend/build" "${STAGE}/frontend/build"
cp "${ROOT}/.env.example" "${STAGE}/.env.example"
chmod 0755 "${STAGE}/csfleet"

tar -C "${STAGE_PARENT}" -czf "${ARCHIVE}" csfleet
echo "${ARCHIVE}"
