package main

import (
	_ "embed"
)

//go:embed config.json
var defaultConfig []byte

//go:embed assets.json
var defaultAssetsCatalog []byte
