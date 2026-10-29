#!/bin/bash
set -euo pipefail

: "${WORK:?WORK is required}"
: "${DEST:?DEST is required}"
test -d "${DEST}"
command -v git >/dev/null
command -v cmake >/dev/null

JOBS="$(sysctl -n hw.logicalcpu 2>/dev/null || echo 8)"
REPO="https://github.com/ggml-org/llama.cpp.git"
SRC="${WORK}/src"
BUILD="${WORK}/build"

rm -rf "${SRC}" "${BUILD}"
REF="$(git ls-remote --tags --refs "${REPO}" | awk -F/ '$NF ~ /^v[0-9]/ {print $NF}' | sort -V | tail -1)"
echo "llama.cpp latest tag: ${REF:-HEAD}"
if [ -n "${REF}" ]; then
  git clone --depth 1 --recurse-submodules --shallow-submodules --branch "${REF}" "${REPO}" "${SRC}"
else
  git clone --depth 1 --recurse-submodules --shallow-submodules "${REPO}" "${SRC}"
fi

cmake_args=(-S "${SRC}" -B "${BUILD}")
if command -v ninja >/dev/null 2>&1; then
  cmake_args+=(-G Ninja)
fi
cmake "${cmake_args[@]}" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DBUILD_SHARED_LIBS=OFF \
  -DGGML_METAL=ON \
  -DGGML_METAL_EMBED_LIBRARY=ON \
  -DGGML_CCACHE=OFF \
  -DGGML_OPENMP=OFF \
  -DLLAMA_OPENSSL=OFF \
  -DLLAMA_CURL=OFF \
  -DLLAMA_BUILD_TESTS=OFF \
  -DLLAMA_BUILD_EXAMPLES=OFF \
  -DLLAMA_BUILD_TOOLS=ON \
  -DLLAMA_BUILD_SERVER=ON
cmake --build "${BUILD}" --config Release --target llama-server --parallel "${JOBS}"

BIN="$(find "${BUILD}" -type f -name llama-server -perm -111 | head -n 1)"
test -n "${BIN}"
install -m 0755 "${BIN}" "${DEST}/llama-server"
strip -x "${DEST}/llama-server" || true
echo "installed ${DEST}/llama-server"
