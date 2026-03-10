package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"noti/internal/domain"
	"noti/internal/infrastructure/downloader"
)

// AssetsService loads and validates assets.json model metadata.
type AssetsService struct {
	basePath       string
	defaultCatalog []byte
}

// NewAssetsService creates a new assets service.
func NewAssetsService(basePath string, defaultCatalog []byte) *AssetsService {
	return &AssetsService{basePath: basePath, defaultCatalog: defaultCatalog}
}

// Load returns the runtime catalog from <basePath>/assets.json, with an
// embedded fallback when runtime file is missing or invalid.
func (s *AssetsService) Load() (*domain.AssetsCatalog, error) {
	runtimePath := filepath.Join(s.basePath, "assets.json")
	data, err := os.ReadFile(runtimePath)
	if err == nil {
		catalog, parseErr := parseAssetsCatalog(data)
		if parseErr == nil {
			return catalog, nil
		}
		slog.Warn("Invalid runtime assets.json; falling back to embedded catalog", "path", runtimePath, "error", parseErr)
	} else if !os.IsNotExist(err) {
		slog.Warn("Failed to read runtime assets.json; falling back to embedded catalog", "path", runtimePath, "error", err)
	} else {
		if mkErr := os.MkdirAll(s.basePath, 0o755); mkErr == nil {
			if writeErr := os.WriteFile(runtimePath, s.defaultCatalog, 0o644); writeErr != nil {
				slog.Warn("Failed to write initial runtime assets.json", "path", runtimePath, "error", writeErr)
			}
		}
	}

	catalog, parseErr := parseAssetsCatalog(s.defaultCatalog)
	if parseErr != nil {
		return nil, fmt.Errorf("embedded assets catalog is invalid: %w", parseErr)
	}
	return catalog, nil
}

// STTRegistryEntries converts catalog STT entries to downloader registry entries.
func (s *AssetsService) STTRegistryEntries(catalog *domain.AssetsCatalog) []downloader.STTModelEntry {
	recommended := make([]downloader.STTModelEntry, 0, 1)
	others := make([]downloader.STTModelEntry, 0, len(catalog.STTModels))
	for _, m := range catalog.STTModels {
		code := strings.TrimSpace(m.ModelCode)
		checksum := strings.TrimSpace(m.Checksums)
		if code != "" {
			entry := downloader.STTModelEntry{Code: code, Checksum: checksum}
			if m.IsRecommended {
				recommended = append(recommended, entry)
			} else {
				others = append(others, entry)
			}
		}
	}
	out := make([]downloader.STTModelEntry, 0, len(recommended)+len(others))
	out = append(out, recommended...)
	out = append(out, others...)
	return out
}

// LLMRegistryEntries converts catalog LLM entries to downloader registry entries.
func (s *AssetsService) LLMRegistryEntries(catalog *domain.AssetsCatalog) []downloader.LLMModelEntry {
	recommended := make([]downloader.LLMModelEntry, 0, 1)
	others := make([]downloader.LLMModelEntry, 0, len(catalog.LLMModels))
	for _, m := range catalog.LLMModels {
		code := strings.TrimSpace(m.ModelCode)
		repo := strings.TrimSpace(m.Repo)
		if code == "" || repo == "" {
			continue
		}

		descriptionParts := make([]string, 0, 3)
		if name := strings.TrimSpace(m.ModelName); name != "" {
			descriptionParts = append(descriptionParts, name)
		}
		if size := strings.TrimSpace(m.Size); size != "" {
			descriptionParts = append(descriptionParts, size)
		}
		if note := strings.TrimSpace(m.Note); note != "" {
			descriptionParts = append(descriptionParts, note)
		}

		description := strings.Join(descriptionParts, " - ")
		if description == "" {
			description = code
		}

		file := code
		if !strings.HasSuffix(strings.ToLower(file), ".gguf") {
			file += ".gguf"
		}

		entry := downloader.LLMModelEntry{
			ModelCode:   code,
			Repo:        repo,
			File:        file,
			Description: description,
		}
		if m.IsRecommended {
			recommended = append(recommended, entry)
		} else {
			others = append(others, entry)
		}
	}
	out := make([]downloader.LLMModelEntry, 0, len(recommended)+len(others))
	out = append(out, recommended...)
	out = append(out, others...)
	return out
}

// RecommendedSTTModel returns the catalog recommended STT model, or first entry.
func (s *AssetsService) RecommendedSTTModel(catalog *domain.AssetsCatalog) string {
	for _, m := range catalog.STTModels {
		if m.IsRecommended {
			if code := strings.TrimSpace(m.ModelCode); code != "" {
				return code
			}
		}
	}
	for _, m := range catalog.STTModels {
		if code := strings.TrimSpace(m.ModelCode); code != "" {
			return code
		}
	}
	return ""
}

// RecommendedLLMModel returns the catalog recommended LLM model, or first entry.
func (s *AssetsService) RecommendedLLMModel(catalog *domain.AssetsCatalog) string {
	for _, m := range catalog.LLMModels {
		if m.IsRecommended {
			if code := strings.TrimSpace(m.ModelCode); code != "" {
				return code
			}
		}
	}
	for _, m := range catalog.LLMModels {
		if code := strings.TrimSpace(m.ModelCode); code != "" {
			return code
		}
	}
	return ""
}

func parseAssetsCatalog(data []byte) (*domain.AssetsCatalog, error) {
	var catalog domain.AssetsCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse assets.json: %w", err)
	}
	if err := validateAssetsCatalog(&catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func validateAssetsCatalog(catalog *domain.AssetsCatalog) error {
	if len(catalog.STTModels) == 0 {
		return fmt.Errorf("stt_models must not be empty")
	}
	if len(catalog.LLMModels) == 0 {
		return fmt.Errorf("llm_models must not be empty")
	}

	seenSTT := make(map[string]struct{}, len(catalog.STTModels))
	for i, model := range catalog.STTModels {
		code := strings.TrimSpace(model.ModelCode)
		if code == "" {
			return fmt.Errorf("stt_models[%d].model_code is required", i)
		}
		if _, ok := seenSTT[code]; ok {
			return fmt.Errorf("duplicate stt model_code %q", code)
		}
		seenSTT[code] = struct{}{}
	}

	seenLLM := make(map[string]struct{}, len(catalog.LLMModels))
	for i, model := range catalog.LLMModels {
		code := strings.TrimSpace(model.ModelCode)
		if code == "" {
			return fmt.Errorf("llm_models[%d].model_code is required", i)
		}
		if strings.TrimSpace(model.Repo) == "" {
			return fmt.Errorf("llm_models[%d].repo is required", i)
		}
		if _, ok := seenLLM[code]; ok {
			return fmt.Errorf("duplicate llm model_code %q", code)
		}
		seenLLM[code] = struct{}{}
	}

	return nil
}
