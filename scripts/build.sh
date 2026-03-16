#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

MODE="${1:-production}"
NOTI_BUILD_PLATFORM="${NOTI_BUILD_PLATFORM:-darwin/arm64}"
NOTI_DMG_NAME="${NOTI_DMG_NAME:-noti.dmg}"

if [[ "$MODE" != "debug" && "$MODE" != "production" ]]; then
  echo "Usage: $0 [debug|production]"
  exit 1
fi

case "$NOTI_BUILD_PLATFORM" in
  darwin/amd64|darwin/arm64)
    ;;
  *)
    echo "Unsupported NOTI_BUILD_PLATFORM: $NOTI_BUILD_PLATFORM"
    echo "Expected one of: darwin/amd64, darwin/arm64"
    exit 1
    ;;
esac

# ── Whisper / CGO environment ────────────────────────────────────────────────
WHISPER_PATH="${WHISPER_PATH:-/Users/a.aryan/Documents/go/github.com/ggerganov/whisper.cpp}"
WHISPER_LIB_DIR="${WHISPER_PATH}/build_go/src"
GGML_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src"
GGML_METAL_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src/ggml-metal"
GGML_BLAS_LIB_DIR="${WHISPER_PATH}/build_go/ggml/src/ggml-blas"

export CGO_LDFLAGS="-L${WHISPER_LIB_DIR} -L${GGML_LIB_DIR} -L${GGML_METAL_LIB_DIR} -L${GGML_BLAS_LIB_DIR} \
  -mmacosx-version-min=13.0"

export CGO_CFLAGS="-I${WHISPER_PATH}/include -I${WHISPER_PATH}/ggml/include -mmacosx-version-min=13.0"
export MACOSX_DEPLOYMENT_TARGET="13.0"
# ─────────────────────────────────────────────────────────────────────────────

if [[ "$MODE" == "debug" ]]; then
  echo "🔨 Building Noti Debug App"
  echo "=========================="
else
  echo "🔨 Building Noti Production App"
  echo "================================"
fi
echo "🎯 Target platform: $NOTI_BUILD_PLATFORM"

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
  wails build -platform "$NOTI_BUILD_PLATFORM" -clean -debug
else
  wails build -platform "$NOTI_BUILD_PLATFORM" -clean -ldflags "-s -w -X main.env=production -X main.sentryDSN=https://cf7b9fda532355b2262930ddbb4d85b6@o4510992653877248.ingest.de.sentry.io/4510992659447888"
fi

# Step 5.1: Optional code signing
SIGNING_IDENTITY="${NOTI_CODESIGN_IDENTITY:-}"
if [[ -n "$SIGNING_IDENTITY" ]]; then
  echo ""
  echo "🔐 Step 5.1: Signing app bundle..."

  ENTITLEMENTS_FILE="build/darwin/Entitlements.plist"
  if [[ "$MODE" == "debug" ]]; then
    ENTITLEMENTS_FILE="build/darwin/Entitlements.dev.plist"
  fi

  CODESIGN_ARGS=(
    --force
    --deep
    --sign "$SIGNING_IDENTITY"
    --entitlements "$ENTITLEMENTS_FILE"
  )

  if [[ "$SIGNING_IDENTITY" != "-" ]]; then
    CODESIGN_ARGS+=(--options runtime --timestamp)
  fi

  codesign "${CODESIGN_ARGS[@]}" "build/bin/noti.app"
  codesign --verify --deep --strict --verbose=2 "build/bin/noti.app"
fi

# Step 5.2: Package production DMG
DMG_PATH=""
if [[ "$MODE" == "production" ]]; then
  echo ""
  echo "💿 Step 5.2: Creating DMG installer..."

  APP_BUNDLE_NAME="noti.app"
  APP_BUNDLE_PATH="build/bin/${APP_BUNDLE_NAME}"
  DMG_NAME="$NOTI_DMG_NAME"
  DMG_PATH="build/bin/${DMG_NAME}"
  DMG_STAGING_DIR="build/bin/dmg-root"
  VOLUME_NAME="Noti"

  if ! command -v create-dmg >/dev/null 2>&1; then
    echo "❌ create-dmg is required to package the installer DMG."
    echo "Install it with: brew install create-dmg"
    exit 1
  fi

  rm -f "$DMG_PATH"
  rm -rf "$DMG_STAGING_DIR"
  mkdir -p "$DMG_STAGING_DIR"

  cp -R "$APP_BUNDLE_PATH" "$DMG_STAGING_DIR/"

  APP_ICON_PATH="${APP_BUNDLE_PATH}/Contents/Resources/iconfile.icns"
  CREATE_DMG_ARGS=(
    --volname "$VOLUME_NAME"
    --no-internet-enable
    --window-pos 120 120
    --window-size 680 420
    --icon-size 112
    --icon "$APP_BUNDLE_NAME" 170 210
    --app-drop-link 510 210
    "$DMG_PATH"
    "$DMG_STAGING_DIR"
  )

  if [[ -f "$APP_ICON_PATH" ]]; then
    CREATE_DMG_ARGS=(--volicon "$APP_ICON_PATH" "${CREATE_DMG_ARGS[@]}")
  fi

  create-dmg "${CREATE_DMG_ARGS[@]}"

  rm -rf "$DMG_STAGING_DIR"
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
  echo "📍 Installer location: build/bin/$NOTI_DMG_NAME"
  echo ""
  echo "To test, run:"
  echo "  ./build/bin/noti.app/Contents/MacOS/noti"
  echo ""
  echo "Or open the app:"
  echo "  open build/bin/noti.app"
  echo ""
  echo "Or open the installer:"
  echo "  open build/bin/$NOTI_DMG_NAME"
fi
