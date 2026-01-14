#!/bin/bash

echo "🔨 Building Noti Debug App"
echo "=========================="

# Step 1: Clean previous builds
echo ""
echo "📦 Step 1: Cleaning previous builds..."
rm -rf build/bin/

# NOTE: We don't clean frontend/dist because the dev build can reuse it.
# If you face issues, uncomment the line below.
# rm -rf frontend/dist/

# Step 1.5: Download the Whisper model download script
echo ""
echo "📜 Step 1.5: Downloading model script..."
mkdir -p scripts
curl -o scripts/download-ggml-model.sh https://raw.githubusercontent.com/ggml-org/whisper.cpp/master/models/download-ggml-model.sh
chmod +x scripts/download-ggml-model.sh

# Step 2: Install frontend dependencies (if needed)
echo ""
echo "📦 Step 2: Installing frontend dependencies..."
cd frontend
bun install
cd ..

# Step 3: Build frontend (if needed)
echo ""
echo "🎨 Step 3: Building frontend..."
cd frontend
bun run build
cd ..

# Step 4: Generate Wails bindings
echo ""
echo "🔗 Step 4: Generating Wails bindings..."
wails generate module

# Step 5: Build the debug app
echo ""
echo "🚀 Step 5: Building debug app..."
# The -debug flag enables the inspector (DevTools)
wails build -platform darwin/arm64 -clean -debug

# Step 6: Verify the build
echo ""
echo "✅ Build complete!"
echo ""
echo "📍 App location: build/bin/noti.app"
echo "To test, run the app and press Option/Alt + Cmd + I to open DevTools."
echo ""
echo "Or run from terminal to see logs:"
echo "  ./build/bin/noti.app/Contents/MacOS/noti"