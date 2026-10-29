#!/bin/bash
set -euo pipefail

: "${WORK:?WORK is required}"
: "${DEST:?DEST is required}"
test -d "${DEST}"
command -v git >/dev/null
command -v cmake >/dev/null

JOBS="$(sysctl -n hw.logicalcpu 2>/dev/null || echo 8)"
REPO="https://github.com/mmwillet/TTS.cpp.git"
SRC="${WORK}/src"
BUILD="${WORK}/build"

rm -rf "${SRC}" "${BUILD}"
git clone --depth 1 --recurse-submodules --shallow-submodules "${REPO}" "${SRC}"
echo "TTS.cpp commit: $(git -C "${SRC}" rev-parse --short HEAD)"

cmake_args=(-S "${SRC}" -B "${BUILD}")
if command -v ninja >/dev/null 2>&1; then
  cmake_args+=(-G Ninja)
fi
cmake "${cmake_args[@]}" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_ARCHITECTURES=arm64 \
  -DBUILD_SHARED_LIBS=OFF \
  -DTTS_BUILD_EXAMPLES=ON \
  -DTTS_CLI_SDL=OFF \
  -DGGML_METAL=ON \
  -DGGML_METAL_EMBED_LIBRARY=ON \
  -DGGML_CCACHE=OFF \
  -DGGML_OPENMP=OFF
cmake --build "${BUILD}" --config Release --target tts-cli --parallel "${JOBS}"

BIN="$(find "${BUILD}" -type f -name tts-cli -perm -111 | head -n 1)"
test -n "${BIN}"
install -m 0755 "${BIN}" "${DEST}/tts.cpp"
strip -x "${DEST}/tts.cpp" || true
echo "installed ${DEST}/tts.cpp"
