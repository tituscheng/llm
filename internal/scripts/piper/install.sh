#!/bin/bash
set -euo pipefail

: "${WORK:?WORK is required}"
: "${DEST:?DEST is required}"
test -d "${DEST}"
command -v git >/dev/null
command -v cmake >/dev/null

JOBS="$(sysctl -n hw.logicalcpu 2>/dev/null || echo 8)"
REPO="https://github.com/OHF-Voice/piper1-gpl.git"
SRC="${WORK}/src"
BUILD="${WORK}/build"

rm -rf "${SRC}" "${BUILD}"
REF="$(git ls-remote --tags --refs "${REPO}" | awk -F/ '$NF ~ /^v[0-9]/ {print $NF}' | sort -V | tail -1)"
echo "piper latest tag: ${REF:-HEAD}"
if [ -n "${REF}" ]; then
  git clone --depth 1 --recurse-submodules --shallow-submodules --branch "${REF}" "${REPO}" "${SRC}"
else
  git clone --depth 1 --recurse-submodules --shallow-submodules "${REPO}" "${SRC}"
fi

python3 -c 'from pathlib import Path; p=Path("'"${WORK}"'/src/libpiper/CMakeLists.txt"); t=p.read_text(); n=t.replace("add_library(piper SHARED","add_library(piper STATIC"); assert n!=t, "failed to patch libpiper to STATIC"; p.write_text(n)'

cmake_args=(-S "${SRC}/libpiper" -B "${BUILD}")
if command -v ninja >/dev/null 2>&1; then
  cmake_args+=(-G Ninja)
fi
cmake "${cmake_args[@]}" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DPIPER_BUILD_TESTS=OFF
cmake --build "${BUILD}" --config Release --target piper_exe --parallel "${JOBS}"

BIN="$(find "${BUILD}" -type f \( -name piper_exe -o -name piper \) -perm -111 | head -n 1)"
test -n "${BIN}"
install -m 0755 "${BIN}" "${DEST}/piper"
DYLIB="$(find "${SRC}/libpiper" -type f -name 'libonnxruntime*.dylib' | head -n 1)"
test -n "${DYLIB}"
DYNAME="$(basename "${DYLIB}")"
install -m 0755 "${DYLIB}" "${DEST}/${DYNAME}"
DATA="$(find "${BUILD}" -type d -name espeak-ng-data | head -n 1)"
test -n "${DATA}"
rm -rf "${DEST}/espeak-ng-data"
cp -R "${DATA}" "${DEST}/espeak-ng-data"
install_name_tool -change "@rpath/${DYNAME}" "@executable_path/${DYNAME}" "${DEST}/piper" || true
install_name_tool -add_rpath "@executable_path" "${DEST}/piper" || true
strip -x "${DEST}/piper" || true
echo "installed ${DEST}/piper (+ ${DYNAME}, espeak-ng-data)"
