package domain

// AssetsCatalog describes the model and binary catalog loaded from assets.json.
type AssetsCatalog struct {
	STTModels   []STTModelAsset  `json:"stt_models"`
	LLMModels   []LLMModelAsset  `json:"llm_models"`
	LlamaServer LlamaServerAsset `json:"llama_server"`
}

// STTModelAsset is a single STT model entry from assets.json.
type STTModelAsset struct {
	ID            int    `json:"id"`
	ModelCode     string `json:"model_code"`
	ModelName     string `json:"model_name"`
	Checksums     string `json:"checksums"`
	IsRecommended bool   `json:"is_recommended"`
	Size          string `json:"size"`
	Note          string `json:"note"`
}

// LLMModelAsset is a single LLM model entry from assets.json.
type LLMModelAsset struct {
	ID            int    `json:"id"`
	ModelCode     string `json:"model_code"`
	ModelName     string `json:"model_name"`
	Checksums     string `json:"checksums"`
	Repo          string `json:"repo"`
	IsRecommended bool   `json:"is_recommended"`
	Size          string `json:"size"`
	Note          string `json:"note"`
}

// LlamaServerAsset contains llama-server source metadata.
type LlamaServerAsset struct {
	Endpoint string `json:"endpoint"`
	Version  string `json:"version"`
}

// ModelOption is a typed API response for STT/LLM model selectors.
type ModelOption struct {
	ID            int    `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	IsRecommended bool   `json:"isRecommended"`
	Note          string `json:"note"`
}
