#!/bin/bash

set -euo pipefail

WHISPER_PATH="${1:-${WHISPER_PATH:-/Users/a.aryan/Documents/go/github.com/ggerganov/whisper.cpp}}"
MIN_MACOS="${MIN_MACOS:-13.0}"
JOBS="${JOBS:-8}"

DEFAULT_ENABLE_METAL="ON"
if [[ "$(uname -m)" == "x86_64" ]]; then
  DEFAULT_ENABLE_METAL="OFF"
fi
WHISPER_ENABLE_METAL="${WHISPER_ENABLE_METAL:-$DEFAULT_ENABLE_METAL}"

if [[ ! -d "$WHISPER_PATH" ]]; then
  echo "Whisper path not found: $WHISPER_PATH"
  echo "Usage: ./scripts/rebuild-whisper.sh [/absolute/path/to/whisper.cpp]"
  exit 1
fi

echo "Rebuilding whisper.cpp static libs"
echo "  Path: $WHISPER_PATH"
echo "  macOS deployment target: $MIN_MACOS"
echo "  Parallel jobs: $JOBS"
echo "  GGML metal backend: $WHISPER_ENABLE_METAL"

cmake -S "$WHISPER_PATH" -B "$WHISPER_PATH/build_go" \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_DEPLOYMENT_TARGET="$MIN_MACOS" \
  -DGGML_METAL="$WHISPER_ENABLE_METAL" \
  -DBUILD_SHARED_LIBS=OFF \
  -DWHISPER_BUILD_TESTS=OFF \
  -DWHISPER_BUILD_EXAMPLES=OFF \
  -DWHISPER_BUILD_SERVER=OFF

cmake --build "$WHISPER_PATH/build_go" -j"$JOBS"

if [[ "$WHISPER_ENABLE_METAL" == "OFF" ]]; then
  GGML_METAL_STUB_DIR="$WHISPER_PATH/build_go/ggml/src/ggml-metal"
  mkdir -p "$GGML_METAL_STUB_DIR"
  rm -f "$GGML_METAL_STUB_DIR/libggml-metal.a"
  ar rcs "$GGML_METAL_STUB_DIR/libggml-metal.a"
fi

# If shared libs exist from older builds, dyld may prefer them.
rm -f "$WHISPER_PATH"/build_go/src/*.dylib
rm -f "$WHISPER_PATH"/build_go/src/*.1
rm -f "$WHISPER_PATH"/build_go/src/*.1.*
rm -f "$WHISPER_PATH"/build_go/ggml/src/*.dylib
rm -f "$WHISPER_PATH"/build_go/ggml/src/*.0
rm -f "$WHISPER_PATH"/build_go/ggml/src/*.0.*
rm -f "$WHISPER_PATH"/build_go/ggml/src/ggml-metal/*.dylib
rm -f "$WHISPER_PATH"/build_go/ggml/src/ggml-metal/*.0
rm -f "$WHISPER_PATH"/build_go/ggml/src/ggml-metal/*.0.*
rm -f "$WHISPER_PATH"/build_go/ggml/src/ggml-blas/*.dylib
rm -f "$WHISPER_PATH"/build_go/ggml/src/ggml-blas/*.0
rm -f "$WHISPER_PATH"/build_go/ggml/src/ggml-blas/*.0.*

echo "Done."
echo "You can now run: ./scripts/build.sh debug"
