package policy

import "noti/internal/domain"

// ShouldDeferModelInitOnStartup indicates whether startup model initialization
// should be deferred until setup is explicitly saved.
func ShouldDeferModelInitOnStartup(isFirstRun bool) bool {
	return isFirstRun
}

// ShouldInitializeSTTOnSave indicates whether SaveConfig should initialize STT.
func ShouldInitializeSTTOnSave(oldModel, newModel string, sttAvailable bool) bool {
	return oldModel != newModel || !sttAvailable
}

// ShouldInitializeLLMOnSave indicates whether SaveConfig should initialize LLM.
func ShouldInitializeLLMOnSave(oldConfig, newConfig domain.LLMConfig, llmAvailable bool) bool {
	if !llmAvailable {
		return true
	}

	return oldConfig.Provider != newConfig.Provider ||
		oldConfig.ModelName != newConfig.ModelName ||
		oldConfig.APIEndpoint != newConfig.APIEndpoint ||
		oldConfig.APIKey != newConfig.APIKey
}
