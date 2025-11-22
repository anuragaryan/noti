package main

import (
	_ "embed"
)

//go:embed scripts/download-ggml-model.sh
var downloadScript []byte

//go:embed config.json
var defaultConfig []byte
