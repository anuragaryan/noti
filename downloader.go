package main

import (
	_ "embed"
)

//go:embed scripts/download-ggml-model.sh
var downloadScript []byte

//go:embed scripts/download-llm-model.sh
var downloadScriptLLM []byte

//go:embed scripts/download-llama-server.sh
var downloadScriptLlamaServer []byte

//go:embed config.json
var defaultConfig []byte
