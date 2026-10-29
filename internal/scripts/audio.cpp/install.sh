#!/bin/bash
set -euo pipefail

: "${WORK:?WORK is required}"
: "${DEST:?DEST is required}"
test -d "${DEST}"
command -v git >/dev/null
command -v cmake >/dev/null

JOBS="$(sysctl -n hw.logicalcpu 2>/dev/null || echo 8)"
REPO="https://github.com/0xShug0/audio.cpp.git"
SRC="${WORK}/src"

rm -rf "${SRC}" "${WORK}/build"
REF="$(git ls-remote --tags --refs "${REPO}" | awk -F/ '{print $NF}' | sort -V | tail -1)"
echo "audio.cpp latest tag: ${REF:-HEAD}"
if [ -n "${REF}" ]; then
  git clone --depth 1 --recurse-submodules --shallow-submodules --branch "${REF}" "${REPO}" "${SRC}"
else
  git clone --depth 1 --recurse-submodules --shallow-submodules "${REPO}" "${SRC}"
fi
echo "audio.cpp commit: $(git -C "${SRC}" rev-parse --short HEAD)"
"${SRC}/scripts/build_metal.sh" \
  --build-dir "${WORK}/build" \
  --build-type Release \
  --archs arm64 \
  --openmp OFF \
  --embed-metal ON \
  --deployment-build \
  --jobs "${JOBS}" \
  --target audiocpp_cli

BIN="$(find "${WORK}/build" -type f -name audiocpp_cli -perm -111 | head -n 1)"
test -n "${BIN}"
install -m 0755 "${BIN}" "${DEST}/audio.cpp"
strip -x "${DEST}/audio.cpp" || true
echo "installed ${DEST}/audio.cpp"
