#!/bin/bash

echo "🔨 Building Noti Production App"
echo "================================"

# Step 1: Clean previous builds
echo ""
echo "📦 Step 1: Cleaning previous builds..."
rm -rf build/bin/
rm -rf frontend/dist/

# Step 2: Install frontend dependencies
echo ""
echo "📦 Step 2: Installing frontend dependencies..."
cd frontend
npm install
cd ..

# Step 3: Build frontend
echo ""
echo "🎨 Step 3: Building frontend..."
cd frontend
npm run build
cd ..

# Step 4: Generate Wails bindings
echo ""
echo "🔗 Step 4: Generating Wails bindings..."
wails generate module

# Step 5: Build the production app
echo ""
echo "🚀 Step 5: Building production app..."
wails build -platform darwin/arm64 -clean -ldflags "-s -w"

# Step 6: Verify the build
echo ""
echo "✅ Build complete!"
echo ""
echo "📍 App location: build/bin/noti.app"
echo "📍 Data will be stored in: ~/Documents/Noti/"
echo ""
echo "To test, run:"
echo "  ./build/bin/noti.app/Contents/MacOS/noti"
echo ""
echo "Or open the app:"
echo "  open build/bin/noti.app"