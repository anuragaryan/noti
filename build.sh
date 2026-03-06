#!/bin/bash

MODE="${1:-production}"

if [[ "$MODE" != "debug" && "$MODE" != "production" ]]; then
  echo "Usage: $0 [debug|production]"
  exit 1
fi

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

if [[ "$MODE" == "debug" ]]; then
  echo "🔨 Building Noti Debug App"
  echo "=========================="
else
  echo "🔨 Building Noti Production App"
  echo "================================"
fi

# Step 1: Clean previous builds
echo ""
echo "📦 Step 1: Cleaning previous builds..."
rm -rf build/bin/

if [[ "$MODE" == "production" ]]; then
  rm -rf frontend/dist/
else
  # NOTE: We don't clean frontend/dist because the dev build can reuse it.
  # If you face issues, uncomment the line below.
  # rm -rf frontend/dist/
  :
fi

# Step 2: Install frontend dependencies
echo ""
echo "📦 Step 2: Installing frontend dependencies..."
cd frontend
bun install
cd ..

# Step 3: Build frontend
echo ""
echo "🎨 Step 3: Building frontend..."
cd frontend
bun run build
cd ..

# Step 4: Generate Wails bindings
echo ""
echo "🔗 Step 4: Generating Wails bindings..."
wails generate module

# Step 5: Build the app
echo ""
echo "🚀 Step 5: Building $MODE app..."
if [[ "$MODE" == "debug" ]]; then
  # The -debug flag enables the inspector (DevTools)
  wails build -platform darwin/arm64 -clean -debug
else
  wails build -platform darwin/arm64 -clean -ldflags "-s -w -X main.env=production -X main.sentryDSN=https://cf7b9fda532355b2262930ddbb4d85b6@o4510992653877248.ingest.de.sentry.io/4510992659447888"
fi

# Step 6: Verify the build
echo ""
echo "✅ Build complete!"
echo ""
echo "📍 App location: build/bin/noti.app"

if [[ "$MODE" == "debug" ]]; then
  echo "To test, run the app and press Option/Alt + Cmd + I to open DevTools."
  echo ""
  echo "Or run from terminal to see logs:"
  echo "  ./build/bin/noti.app/Contents/MacOS/noti"
else
  echo "📍 Data will be stored in: ~/Documents/noti/"
  echo ""
  echo "To test, run:"
  echo "  ./build/bin/noti.app/Contents/MacOS/noti"
  echo ""
  echo "Or open the app:"
  echo "  open build/bin/noti.app"
fi
