package main

import (
	_ "embed"
)

//go:embed config.json
var defaultConfig []byte
