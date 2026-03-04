#!/bin/bash

# ── Whisper / CGO environment ────────────────────────────────────────────────
WHISPER_PATH="/Users/a.aryan/Documents/go/github.com/ggerganov/whisper.cpp"
WHISPER_LIB_DIR="${WHISPER_PATH}/build_go/src"
GGML_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src"
GGML_METAL_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src/ggml-metal"
GGML_BLAS_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src/ggml-blas"

export CGO_LDFLAGS="-L${WHISPER_LIB_DIR} -L${GGML_LIB_DIR} -L${GGML_METAL_LIB_DIR} -L${GGML_BLAS_LIB_DIR} \
  -lwhisper -lggml -lggml-metal -lggml-blas -lportaudio \
  -framework Accelerate -framework Foundation -framework Metal"

export CGO_CFLAGS="-I${WHISPER_PATH}/include -I${WHISPER_PATH}/ggml/include"
# ─────────────────────────────────────────────────────────────────────────────

PACKAGE="${1:-./...}"

echo "🧪 Running tests: $PACKAGE"
echo "=========================="
go test -v -race "$PACKAGE"
