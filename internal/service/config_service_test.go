package service

import (
	"os"
	"path/filepath"
	"testing"
)

const testDefaultConfigJSON = `{
  "modelName": "base.en",
  "llm": {
    "provider": "local",
    "modelName": "gemma",
    "apiEndpoint": "",
    "apiKey": "",
    "temperature": 0.7,
    "maxTokens": 512
  },
  "audio": {
    "defaultSource": "microphone",
    "mixer": {
      "microphoneGain": 1.0,
      "systemGain": 1.0,
      "mixMode": "sum"
    }
  }
}`

func TestConfigService_Load_PreservesExplicitZeroValues(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	configPath := filepath.Join(basePath, "config.json")

	content := `{
  "modelName": "base.en",
  "llm": {
    "provider": "local",
    "modelName": "gemma",
    "temperature": 0,
    "maxTokens": 512
  },
  "audio": {
    "defaultSource": "mixed",
    "mixer": {
      "microphoneGain": 0,
      "systemGain": 0,
      "mixMode": "sum"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc := NewConfigService(basePath, []byte(testDefaultConfigJSON))
	config, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if config.LLM.Temperature != 0 {
		t.Fatalf("temperature should preserve explicit zero, got %v", config.LLM.Temperature)
	}
	if config.Audio.Mixer.MicrophoneGain != 0 {
		t.Fatalf("microphoneGain should preserve explicit zero, got %v", config.Audio.Mixer.MicrophoneGain)
	}
	if config.Audio.Mixer.SystemGain != 0 {
		t.Fatalf("systemGain should preserve explicit zero, got %v", config.Audio.Mixer.SystemGain)
	}
}

func TestConfigService_Load_AppliesDefaultsWhenFieldsMissing(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	configPath := filepath.Join(basePath, "config.json")

	content := `{
  "modelName": "base.en",
  "llm": {
    "provider": "local",
    "modelName": "gemma",
    "maxTokens": 512
  },
  "audio": {
    "defaultSource": "mixed",
    "mixer": {
      "mixMode": "sum"
    }
  }
}`

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	svc := NewConfigService(basePath, []byte(testDefaultConfigJSON))
	config, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if config.LLM.Temperature != 0.7 {
		t.Fatalf("temperature should default to 0.7 when missing, got %v", config.LLM.Temperature)
	}
	if config.Audio.Mixer.MicrophoneGain != 1.0 {
		t.Fatalf("microphoneGain should default to 1.0 when missing, got %v", config.Audio.Mixer.MicrophoneGain)
	}
	if config.Audio.Mixer.SystemGain != 1.0 {
		t.Fatalf("systemGain should default to 1.0 when missing, got %v", config.Audio.Mixer.SystemGain)
	}
}
