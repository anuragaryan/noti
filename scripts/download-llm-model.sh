#!/bin/bash

# Download script for LLM models (GGUF format)
# Usage: ./download-llm-model.sh <model-name>
# Example: ./download-llm-model.sh gemma-3-1b-it-q4_k_m

set -e

MODEL_NAME="$1"

if [ -z "$MODEL_NAME" ]; then
    echo "Usage: $0 <model-name>"
    echo "Example: $0 gemma-3-1b-it-q4_k_m"
    exit 1
fi

# Model file name
MODEL_FILE="${MODEL_NAME}.gguf"

# Check if model already exists
if [ -f "$MODEL_FILE" ]; then
    echo "Model $MODEL_FILE already exists. Skipping download."
    exit 0
fi

echo "Downloading LLM model: $MODEL_NAME"
echo "This may take a while depending on your internet connection..."

# Map common model names to Hugging Face repositories
case "$MODEL_NAME" in
    gemma-3-270m-it-q4_k_m|gemma-3-270m-it)
        REPO="ggml-org/gemma-3-270m-it-GGUF"
        FILE="gemma-3-270m-it-Q8_0.gguf"
        ;;
    gemma-3-1b-it-q4_k_m|gemma-3-1b-it)
        REPO="ggml-org/gemma-3-1b-it-GGUF"
        FILE="gemma-3-1b-it-Q4_K_M.gguf"
        ;;
    gemma-2-2b-it-q4_k_m|gemma-2-2b-it)
        REPO="ggml-org/gemma-2-2b-it-GGUF"
        FILE="gemma-2-2b-it-Q4_K_M.gguf"
        ;;
    gemma-2-9b-it-q4_k_m|gemma-2-9b-it)
        REPO="ggml-org/gemma-2-9b-it-GGUF"
        FILE="gemma-2-9b-it-Q4_K_M.gguf"
        ;;
    llama-2-7b-q4_k_m|llama-2-7b)
        REPO="TheBloke/Llama-2-7B-GGUF"
        FILE="llama-2-7b.Q4_K_M.gguf"
        ;;
    *)
        echo "Unknown model: $MODEL_NAME"
        echo "Supported models:"
        echo "  - gemma-3-270m-it-q4_k_m (Google Gemma 3 270M Instruct, Q4_K_M quantization)"
        echo "  - gemma-3-1b-it-q4_k_m (Google Gemma 3 1B Instruct, Q4_K_M quantization)"
        echo "  - gemma-2-2b-it-q4_k_m (Google Gemma 2 2B Instruct, Q4_K_M quantization)"
        echo "  - gemma-2-9b-it-q4_k_m (Google Gemma 2 9B Instruct, Q4_K_M quantization)"
        echo "  - llama-2-7b-q4_k_m (Llama 2 7B, Q4_K_M quantization)"
        exit 1
        ;;
esac

# Construct Hugging Face download URL
HF_URL="https://huggingface.co/${REPO}/resolve/main/${FILE}"

echo "Downloading from: $HF_URL"
echo "Saving to: $MODEL_FILE"

# Download using curl with progress bar
if command -v curl &> /dev/null; then
    curl -L -o "$MODEL_FILE" --progress-bar "$HF_URL"
elif command -v wget &> /dev/null; then
    wget -O "$MODEL_FILE" --show-progress "$HF_URL"
else
    echo "Error: Neither curl nor wget is available. Please install one of them."
    exit 1
fi

# Verify the download
if [ -f "$MODEL_FILE" ]; then
    FILE_SIZE=$(du -h "$MODEL_FILE" | cut -f1)
    echo "✓ Download complete!"
    echo "✓ Model saved to: $MODEL_FILE"
    echo "✓ File size: $FILE_SIZE"
else
    echo "✗ Download failed!"
    exit 1
fi