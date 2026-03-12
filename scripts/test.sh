#!/bin/bash

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# ── Whisper / CGO environment ────────────────────────────────────────────────
WHISPER_PATH="/Users/a.aryan/Documents/go/github.com/ggerganov/whisper.cpp"
WHISPER_LIB_DIR="${WHISPER_PATH}/build_go/src"
GGML_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src"
GGML_METAL_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src/ggml-metal"
GGML_BLAS_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src/ggml-blas"

export CGO_LDFLAGS="-L${WHISPER_LIB_DIR} -L${GGML_LIB_DIR} -L${GGML_METAL_LIB_DIR} -L${GGML_BLAS_LIB_DIR} \
  -mmacosx-version-min=13.0"

export CGO_CFLAGS="-I${WHISPER_PATH}/include -I${WHISPER_PATH}/ggml/include -mmacosx-version-min=13.0"
export MACOSX_DEPLOYMENT_TARGET="13.0"
# ─────────────────────────────────────────────────────────────────────────────

NO_CACHE=""
PACKAGE="./..."

while [[ "$#" -gt 0 ]]; do
    case $1 in
        --no-cache) NO_CACHE="-count=1"; shift ;;
        -h|--help)
            echo "Usage: ./scripts/test.sh [OPTIONS] [PACKAGE]"
            echo "Options:"
            echo "  --no-cache    Run tests without cache (adds -count=1)"
            echo "  -h, --help    Show this help message"
            exit 0
            ;;
        *) PACKAGE="$1"; shift ;;
    esac
done

echo "🧪 Running tests: $PACKAGE"
echo "=========================="
go test -v -race $NO_CACHE "$PACKAGE"
