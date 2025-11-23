#!/bin/bash

# Download script for llama-server binaries
# This script downloads the appropriate llama-server binary for the current platform

set -e

# Detect platform
OS=$(uname -s)
ARCH=$(uname -m)

# Map architecture names
case "$ARCH" in
    x86_64)
        ARCH="x64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Map OS names and set binary name
case "$OS" in
    Darwin)
        PLATFORM="macos"
        BINARY_NAME="llama-server"
        ;;
    Linux)
        PLATFORM="linux"
        BINARY_NAME="llama-server"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        PLATFORM="windows"
        BINARY_NAME="llama-server.exe"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Check if binary already exists
if [ -f "$BINARY_NAME" ]; then
    echo "llama-server binary already exists. Skipping download."
    exit 0
fi

echo "Downloading llama-server for $PLATFORM-$ARCH..."

# llama.cpp releases URL
REPO="ggerganov/llama.cpp"

# Try to get latest release tag from GitHub API
LATEST_TAG=$(curl -s -H "Accept: application/vnd.github.v3+json" "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

# Fallback to a known stable version if API fails
if [ -z "$LATEST_TAG" ]; then
    echo "Warning: Could not fetch latest release from GitHub API, using fallback version"
    # Use a recent stable version that has pre-built binaries
    LATEST_TAG="b7134"
fi

echo "Using release: $LATEST_TAG"

# Construct download URL based on platform
case "$PLATFORM-$ARCH" in
    macos-arm64)
        ASSET_NAME="llama-${LATEST_TAG}-bin-macos-arm64.zip"
        ;;
    macos-x64)
        ASSET_NAME="llama-${LATEST_TAG}-bin-macos-x64.zip"
        ;;
    linux-x64)
        ASSET_NAME="llama-${LATEST_TAG}-bin-ubuntu-x64.zip"
        ;;
    linux-arm64)
        ASSET_NAME="llama-${LATEST_TAG}-bin-ubuntu-arm64.zip"
        ;;
    windows-x64)
        ASSET_NAME="llama-${LATEST_TAG}-bin-win-llvm-x64.zip"
        ;;
    windows-arm64)
        ASSET_NAME="llama-${LATEST_TAG}-bin-win-llvm-arm64.zip"
        ;;
    *)
        echo "No pre-built binary available for $PLATFORM-$ARCH"
        exit 1
        ;;
esac

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$ASSET_NAME"

# Clean up any existing incomplete download
if [ -f "llama-server.zip" ]; then
    echo "Found existing llama-server.zip, checking if valid..."
    if file "llama-server.zip" 2>/dev/null | grep -q "Zip archive"; then
        echo "Existing zip file is valid, using it"
    else
        echo "Existing zip file is invalid or incomplete, removing..."
        rm -f "llama-server.zip"
    fi
fi

# Download if not already present
if [ ! -f "llama-server.zip" ]; then
    echo "Downloading from: $DOWNLOAD_URL"
    
    # Download the archive
    if command -v curl &> /dev/null; then
        if ! curl -L -f -o "llama-server.zip" "$DOWNLOAD_URL"; then
            echo "Error: Download failed"
            rm -f "llama-server.zip"
            exit 1
        fi
    elif command -v wget &> /dev/null; then
        if ! wget -O "llama-server.zip" "$DOWNLOAD_URL"; then
            echo "Error: Download failed"
            rm -f "llama-server.zip"
            exit 1
        fi
    else
        echo "Error: Neither curl nor wget is available"
        exit 1
    fi
    
    echo "Download complete, verifying..."
    
    # Verify the downloaded file
    if ! file "llama-server.zip" 2>/dev/null | grep -q "Zip archive"; then
        echo "Error: Downloaded file is not a valid zip archive"
        echo "File info: $(file llama-server.zip 2>/dev/null || echo 'file command failed')"
        rm -f "llama-server.zip"
        exit 1
    fi
    
    echo "✓ Download verified successfully"
fi

# Extract the binary and dependencies
echo "Extracting llama-server from archive..."
if command -v unzip &> /dev/null; then
    # Extract the entire bin directory to preserve dependencies
    if ! unzip -j "llama-server.zip" "*/bin/*" -d . 2>/dev/null; then
        # Fallback: try extracting just the binary
        if ! unzip -j "llama-server.zip" "*/$BINARY_NAME" -d . 2>/dev/null; then
            if ! unzip -j "llama-server.zip" "$BINARY_NAME" -d . 2>/dev/null; then
                echo "Error: Could not find $BINARY_NAME in the archive"
                echo "Archive contents:"
                unzip -l "llama-server.zip" | head -20
                rm -f "llama-server.zip"
                exit 1
            fi
        fi
    fi
    
    # Also try to extract any .dylib or .so files (shared libraries)
    unzip -j "llama-server.zip" "*/bin/*.dylib" -d . 2>/dev/null || true
    unzip -j "llama-server.zip" "*/bin/*.so" -d . 2>/dev/null || true
    unzip -j "llama-server.zip" "*/bin/*.dll" -d . 2>/dev/null || true
else
    echo "Error: unzip is not available"
    rm -f "llama-server.zip"
    exit 1
fi

# Clean up
rm -f "llama-server.zip"

# Make executable (Unix-like systems)
if [ "$PLATFORM" != "windows" ]; then
    chmod +x "$BINARY_NAME"
fi

# Verify the binary
if [ -f "$BINARY_NAME" ]; then
    FILE_SIZE=$(du -h "$BINARY_NAME" | cut -f1)
    echo "✓ Download complete!"
    echo "✓ Binary saved to: $BINARY_NAME"
    echo "✓ File size: $FILE_SIZE"
else
    echo "✗ Download failed!"
    exit 1
fi