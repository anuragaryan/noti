package policy

import (
	"testing"

	"noti/internal/domain"
)

func TestShouldDeferModelInitOnStartup(t *testing.T) {
	if !ShouldDeferModelInitOnStartup(true) {
		t.Fatal("expected first run to defer model initialization")
	}

	if ShouldDeferModelInitOnStartup(false) {
		t.Fatal("expected non-first run to initialize models during startup")
	}
}

func TestShouldInitializeSTTOnSave(t *testing.T) {
	if !ShouldInitializeSTTOnSave("large-v3", "large-v3", "en", "en", false) {
		t.Fatal("expected STT initialization when service unavailable")
	}

	if !ShouldInitializeSTTOnSave("large-v2", "large-v3", "en", "en", true) {
		t.Fatal("expected STT initialization when model changes")
	}

	if !ShouldInitializeSTTOnSave("large-v3", "large-v3", "en", "es", true) {
		t.Fatal("expected STT initialization when language changes")
	}

	if ShouldInitializeSTTOnSave("large-v3", "large-v3", "en", "en", true) {
		t.Fatal("did not expect STT initialization when model unchanged and available")
	}
}

func TestShouldInitializeLLMOnSave(t *testing.T) {
	oldCfg := domain.LLMConfig{
		Provider:    "local",
		ModelName:   "Qwen3.5-4B-UD-Q4_K_XL",
		APIEndpoint: "",
		APIKey:      "",
	}

	if !ShouldInitializeLLMOnSave(oldCfg, oldCfg, false) {
		t.Fatal("expected LLM initialization when service unavailable")
	}

	newCfg := oldCfg
	newCfg.ModelName = "Qwen3.5-8B-Q4"
	if !ShouldInitializeLLMOnSave(oldCfg, newCfg, true) {
		t.Fatal("expected LLM initialization when model changes")
	}

	if ShouldInitializeLLMOnSave(oldCfg, oldCfg, true) {
		t.Fatal("did not expect LLM initialization when config unchanged and available")
	}
}
