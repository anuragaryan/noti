package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"noti/internal/domain"
	"noti/internal/infrastructure/downloader"
	"noti/internal/policy"
	"noti/internal/repository"
	"noti/internal/service"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx           context.Context
	eventsEnabled bool
	streamMu      sync.Mutex
	streamCancel  context.CancelFunc
	streamID      uint64
	basePath      string
	structurePath string
	notesPath     string
	config        *domain.Config
	assetsCatalog *domain.AssetsCatalog
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	fileSystem    *repository.FileSystem
	folderService *service.FolderService
	noteService   *service.NoteService
	configService *service.ConfigService
	assetsService *service.AssetsService
	sttManager    *service.STTManager
	llmManager    *service.LLMManager
	promptService *service.PromptService
	audioManager  *service.AudioManager
}

type apiModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func normalizeModelsEndpoint(rawEndpoint string) (string, error) {
	trimmed := strings.TrimSpace(rawEndpoint)
	if trimmed == "" {
		return "", fmt.Errorf("API endpoint is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid API endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("API endpoint must start with http:// or https://")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("API endpoint host is required")
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	switch {
	case path == "":
		path = "/v1/models"
	case strings.HasSuffix(path, "/chat/completions"):
		path = strings.TrimSuffix(path, "/chat/completions") + "/models"
	case strings.HasSuffix(path, "/models"):
		// Already points to models endpoint.
	case strings.HasSuffix(path, "/v1"):
		path = path + "/models"
	default:
		path = path + "/v1/models"
	}

	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// NewApp creates a new App application struct
func NewApp() *App {
	configDir, err := os.UserConfigDir()
	if err != nil {
		slog.Error("could not determine user config directory", "err", err)
		os.Exit(1)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Error("could not determine user home directory", "err", err)
		os.Exit(1)
	}

	basePath := filepath.Join(configDir, "Noti")
	notesPath := filepath.Join(homeDir, "Documents", "noti", "notes")
	structurePath := filepath.Join(notesPath, "structure.json")

	// Initialize repositories
	structureRepo := repository.NewStructureRepository(structurePath)
	pathResolver := repository.NewPathResolver(notesPath)
	fileSystem := repository.NewFileSystem(pathResolver)

	// Initialize services
	folderService := service.NewFolderService(structureRepo, pathResolver, notesPath)
	noteService := service.NewNoteService(structureRepo, pathResolver, fileSystem, notesPath)
	configService := service.NewConfigService(basePath, defaultConfig)
	assetsService := service.NewAssetsService(basePath, defaultAssetsCatalog)
	sttManager := service.NewSTTManager(basePath)
	llmManager := service.NewLLMManager(basePath)
	promptService := service.NewPromptService(basePath)
	audioManager := service.NewAudioManager()

	// Connect audio manager to STT manager
	sttManager.SetAudioManager(audioManager)

	return &App{
		eventsEnabled: true,
		basePath:      basePath,
		structurePath: structurePath,
		notesPath:     notesPath,
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		fileSystem:    fileSystem,
		folderService: folderService,
		noteService:   noteService,
		configService: configService,
		assetsService: assetsService,
		sttManager:    sttManager,
		llmManager:    llmManager,
		promptService: promptService,
		audioManager:  audioManager,
	}
}

func (a *App) emitEvent(name string, payload ...interface{}) {
	if !a.eventsEnabled || a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, name, payload...)
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Create notes directory if it doesn't exist
	if err := os.MkdirAll(a.notesPath, 0755); err != nil {
		slog.Error("Cannot create notes directory", "error", err, "path", a.notesPath)
		slog.Error("Please check that the application has permission to write to the Documents folder.")
		return
	}

	// Test write permissions by creating a test file
	testFile := filepath.Join(a.notesPath, ".permission_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		slog.Error("Cannot write to notes directory", "error", err, "path", a.notesPath)
		slog.Error("The application does not have write permissions.")
		return
	}
	os.Remove(testFile)
	slog.Info("Successfully initialized notes directory", "path", a.notesPath)

	// Load config
	recommendedSTT := ""
	recommendedLLM := ""

	catalog, err := a.assetsService.Load()
	if err != nil {
		slog.Warn("Model catalog unavailable; continuing with config-only defaults", "error", err)
		a.assetsCatalog = nil
		downloader.SetSTTRegistryUnavailable(err.Error())
		downloader.SetLLMRegistryUnavailable(err.Error())
	} else {
		a.assetsCatalog = catalog

		downloader.SetSTTRegistry(a.assetsService.STTRegistryEntries(catalog))
		downloader.SetLLMRegistry(a.assetsService.LLMRegistryEntries(catalog))

		recommendedSTT = a.assetsService.RecommendedSTTModel(catalog)
		recommendedLLM = a.assetsService.RecommendedLLMModel(catalog)
		a.configService.SetModelDefaults(recommendedSTT, recommendedLLM)
	}

	config, err := a.configService.Load()
	if err != nil {
		slog.Error("Cannot load config", "error", err)
		// Use a default config if loading fails
		a.config = &domain.Config{ModelName: recommendedSTT, STTLanguage: "en"}
		a.config.LLM.Provider = "local"
		a.config.LLM.ModelName = recommendedLLM
		a.config.Audio = domain.DefaultAudioSettings()
		slog.Warn("Using default STT config.")
	} else {
		a.config = config
	}

	// Initialize Audio Manager
	a.audioManager.SetContext(ctx)
	if err := a.audioManager.Initialize(); err != nil {
		slog.Error("Audio Manager initialization failed", "error", err)
		slog.Warn("Audio capture features may be limited.")
	}

	// Apply audio source from config after audio manager is initialized
	audioSource := domain.AudioSourceFromString(a.config.Audio.DefaultSource)
	if err := a.audioManager.SetAudioSource(audioSource); err != nil {
		slog.Warn("Failed to set audio source from config", "error", err)
	} else {
		a.sttManager.SetAudioSource(audioSource)
		slog.Info("Audio source set", "source", a.config.Audio.DefaultSource)
	}

	// Defer model initialization on first run until the user saves Getting Started.
	deferModelInit := policy.ShouldDeferModelInitOnStartup(a.configService.IsFirstRun())

	// Initialize STT service with self-healing
	a.sttManager.SetContext(ctx)
	if deferModelInit {
		slog.Info("First run detected; deferring STT initialization until setup is saved")
	} else {
		sttConfig := &domain.STTConfig{
			ModelName: a.config.ModelName,
			Language:  a.config.STTLanguage,
		}
		if err := a.sttManager.Initialize(sttConfig); err != nil {
			slog.Error("STT initialization failed", "error", err)
		}
	}

	// Initialize LLM service if configured
	a.llmManager.SetContext(ctx)
	if deferModelInit {
		slog.Info("First run detected; deferring LLM initialization until setup is saved")
	} else if a.config.LLM.Provider != "" {
		if err := a.llmManager.Initialize(&a.config.LLM); err != nil {
			slog.Error("LLM initialization failed", "error", err)
			slog.Warn("LLM features will be disabled. You can configure LLM settings later.")
		}
	} else {
		slog.Info("LLM not configured. You can enable it in settings.")
	}

	// Initialize prompt service
	if err := a.promptService.Initialize(); err != nil {
		slog.Error("Prompt service initialization failed", "error", err)
	}

	// Create structure.json if it doesn't exist
	if _, err := os.Stat(a.structurePath); os.IsNotExist(err) {
		if err := a.structureRepo.Save(&domain.FolderStructure{
			Folders: []domain.Folder{},
			Notes:   []domain.Note{},
		}); err != nil {
			slog.Error("Cannot create structure.json", "error", err)
			return
		}
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	a.cancelActiveStream()
	a.sttManager.Cleanup()
	a.llmManager.Cleanup()
	if a.audioManager != nil {
		a.audioManager.Cleanup()
	}
}

// ============================================================================
// STT OPERATIONS
// ============================================================================

// StartVoiceRecording starts recording audio from the current audio source
func (a *App) StartVoiceRecording() error {
	if !a.sttManager.IsAvailable() {
		return fmt.Errorf("STT service not available. Please download the STT model")
	}
	// Use the new audio source-aware recording
	return a.sttManager.StartRecordingWithSource(a.sttManager.GetAudioSource())
}

// StartVoiceRecordingWithSource starts recording with a specific audio source
func (a *App) StartVoiceRecordingWithSource(source string) error {
	if !a.sttManager.IsAvailable() {
		return fmt.Errorf("STT service not available. Please download the STT model")
	}
	audioSource := domain.AudioSourceFromString(source)
	return a.sttManager.StartRecordingWithSource(audioSource)
}

// StopVoiceRecording stops recording and returns transcribed text
func (a *App) StopVoiceRecording() (*domain.TranscriptionResult, error) {
	if !a.sttManager.IsAvailable() {
		return nil, fmt.Errorf("STT service not available")
	}
	return a.sttManager.StopRecordingWithTranscription()
}

// IsRecording returns current recording status
func (a *App) IsRecording() bool {
	if a.audioManager != nil && a.audioManager.IsCapturing() {
		return true
	}

	if !a.sttManager.IsAvailable() {
		return false
	}

	svc := a.sttManager.GetService()
	if svc != nil {
		return svc.IsProcessing()
	}

	return false
}

// GetSTTStatus returns whether STT is available
func (a *App) GetSTTStatus() map[string]interface{} {
	available := a.sttManager.IsAvailable()
	modelFileName := "N/A"
	if a.config != nil {
		modelFileName = fmt.Sprintf("ggml-%s.bin", a.config.ModelName)
	}
	modelPath := filepath.Join(a.basePath, "models", modelFileName)

	return map[string]interface{}{
		"available": available,
		"modelPath": modelPath,
	}
}

// GetSTTModels returns STT model options sorted by catalog ID.
func (a *App) GetSTTModels() []domain.ModelOption {
	if a.assetsCatalog == nil {
		return []domain.ModelOption{}
	}

	models := append([]domain.STTModelAsset(nil), a.assetsCatalog.STTModels...)
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	out := make([]domain.ModelOption, 0, len(models))
	for _, model := range models {
		out = append(out, domain.ModelOption{
			ID:            model.ID,
			Code:          model.ModelCode,
			Name:          model.ModelName,
			IsRecommended: model.IsRecommended,
			Note:          model.Note,
		})
	}

	return out
}

// ============================================================================
// AUDIO SOURCE OPERATIONS
// ============================================================================

// GetAudioSources returns available audio sources
func (a *App) GetAudioSources() []map[string]interface{} {
	sources := a.sttManager.GetAvailableAudioSources()
	result := make([]map[string]interface{}, len(sources))
	for i, source := range sources {
		result[i] = map[string]interface{}{
			"id":   source.String(),
			"name": source.DisplayName(),
		}
	}
	return result
}

// GetCurrentAudioSource returns the current audio source
func (a *App) GetCurrentAudioSource() string {
	return a.sttManager.GetAudioSource().String()
}

// SetAudioSource sets the audio source for recording
func (a *App) SetAudioSource(source string) error {
	audioSource := domain.AudioSourceFromString(source)
	return a.sttManager.SetAudioSource(audioSource)
}

// GetAudioDevices returns all available audio devices
func (a *App) GetAudioDevices() ([]domain.AudioDevice, error) {
	if a.audioManager == nil {
		return nil, fmt.Errorf("audio manager not initialized")
	}
	return a.audioManager.GetAvailableDevices()
}

// CheckAudioPermissions checks permissions for the specified audio source
func (a *App) CheckAudioPermissions(source string) map[string]interface{} {
	if a.audioManager == nil {
		return map[string]interface{}{
			"status":  "unknown",
			"message": "Audio manager not initialized",
		}
	}
	audioSource := domain.AudioSourceFromString(source)
	status := a.audioManager.CheckPermissions(audioSource)
	return map[string]interface{}{
		"status":  status.String(),
		"granted": status == domain.PermissionGranted,
	}
}

// RequestAudioPermissions requests permissions for the specified audio source
func (a *App) RequestAudioPermissions(source string) error {
	if a.audioManager == nil {
		return fmt.Errorf("audio manager not initialized")
	}
	audioSource := domain.AudioSourceFromString(source)
	return a.audioManager.RequestPermissions(audioSource)
}

// GetMixerConfig returns the current audio mixer configuration
func (a *App) GetMixerConfig() map[string]interface{} {
	if a.audioManager == nil {
		return map[string]interface{}{}
	}
	config := a.audioManager.GetMixerConfig()
	return map[string]interface{}{
		"microphoneGain": config.MicrophoneGain,
		"systemGain":     config.SystemGain,
		"mixMode":        config.MixMode,
	}
}

// SetMixerConfig updates the audio mixer configuration
func (a *App) SetMixerConfig(micGain, sysGain float32, mixMode string) {
	if a.audioManager == nil {
		return
	}
	config := domain.AudioMixerConfig{
		MicrophoneGain: micGain,
		SystemGain:     sysGain,
		MixMode:        mixMode,
	}
	a.audioManager.SetMixerConfig(config)
}

// GetAudioStatus returns comprehensive audio system status
func (a *App) GetAudioStatus() map[string]interface{} {
	status := map[string]interface{}{
		"initialized":   a.audioManager != nil,
		"currentSource": a.sttManager.GetAudioSource().String(),
		"isCapturing":   false,
	}

	if a.audioManager != nil {
		status["isCapturing"] = a.audioManager.IsCapturing()
		status["availableSources"] = a.GetAudioSources()

		// Check permissions for each source
		permissions := make(map[string]string)
		for _, source := range a.sttManager.GetAvailableAudioSources() {
			perm := a.audioManager.CheckPermissions(source)
			permissions[source.String()] = perm.String()
		}
		status["permissions"] = permissions
	}

	return status
}

// ============================================================================
// LLM OPERATIONS
// ============================================================================

// GenerateText generates text using the configured LLM
func (a *App) GenerateText(prompt string, systemPrompt string) (*domain.LLMResponse, error) {
	slog.Debug("GenerateText called", "promptLength", len(prompt), "llmAvailable", a.llmManager.IsAvailable())

	if !a.llmManager.IsAvailable() {
		status := a.llmManager.GetStatus()
		slog.Warn("GenerateText: LLM service not available", "status", status)
		return nil, fmt.Errorf("LLM service not available. Please configure LLM settings")
	}

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}

	slog.Debug("GenerateText: calling llmManager.Generate")
	return a.llmManager.Generate(a.ctx, request)
}

// GetLLMStatus returns LLM availability and configuration
func (a *App) GetLLMStatus() map[string]interface{} {
	status := a.llmManager.GetStatus()

	// Add detailed debugging info
	if provider := a.llmManager.GetProvider(); provider != nil {
		status["providerAvailable"] = provider.IsAvailable()
		status["providerInfo"] = provider.GetModelInfo()
	} else {
		status["providerAvailable"] = false
		status["providerInfo"] = nil
	}

	slog.Debug("LLM Status", "status", status)
	return status
}

// GetLLMModels returns LLM model options sorted by catalog ID.
func (a *App) GetLLMModels() []domain.ModelOption {
	if a.assetsCatalog == nil {
		return []domain.ModelOption{}
	}

	models := append([]domain.LLMModelAsset(nil), a.assetsCatalog.LLMModels...)
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})

	out := make([]domain.ModelOption, 0, len(models))
	for _, model := range models {
		out = append(out, domain.ModelOption{
			ID:            model.ID,
			Code:          model.ModelCode,
			Name:          model.ModelName,
			IsRecommended: model.IsRecommended,
			Note:          model.Note,
		})
	}

	return out
}

// GetAPILLMModels returns model options discovered from an OpenAI-compatible API domain.
func (a *App) GetAPILLMModels(apiEndpoint string, apiKey string) ([]domain.ModelOption, error) {
	modelsEndpoint, err := normalizeModelsEndpoint(apiEndpoint)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	trimmedAPIKey := strings.TrimSpace(apiKey)
	if trimmedAPIKey != "" {
		bearer := trimmedAPIKey
		if !strings.HasPrefix(strings.ToLower(bearer), "bearer ") {
			bearer = "Bearer " + trimmedAPIKey
		}
		req.Header.Set("Authorization", bearer)
		req.Header.Set("api-key", trimmedAPIKey)
		req.Header.Set("x-api-key", trimmedAPIKey)
	}

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to load API models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read API models response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiErrorResponse
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Error.Message)
		}
		preview := strings.TrimSpace(string(body))
		if len(preview) > 220 {
			preview = preview[:220] + "..."
		}
		return nil, fmt.Errorf("API models request failed (%d): %s", resp.StatusCode, preview)
	}

	var decoded apiModelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		preview := strings.TrimSpace(string(body))
		if len(preview) > 220 {
			preview = preview[:220] + "..."
		}
		return nil, fmt.Errorf("failed to decode API models response: %w (response preview: %s)", err, preview)
	}

	ids := make([]string, 0, len(decoded.Data))
	seen := make(map[string]struct{}, len(decoded.Data))
	for _, model := range decoded.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		left := strings.ToLower(ids[i])
		right := strings.ToLower(ids[j])
		if left == right {
			return ids[i] < ids[j]
		}
		return left < right
	})

	out := make([]domain.ModelOption, 0, len(ids))
	for idx, id := range ids {
		out = append(out, domain.ModelOption{
			ID:            idx + 1,
			Code:          id,
			Name:          id,
			IsRecommended: false,
			Note:          "",
		})
	}

	return out, nil
}

// UpdateLLMConfig updates LLM configuration and switches provider
func (a *App) UpdateLLMConfig(llmConfig domain.LLMConfig) error {
	if llmConfig.MaxTokens < 50 {
		return fmt.Errorf("LLM max tokens must be more than 50")
	}
	if llmConfig.MaxTokens > 1_000_000 {
		return fmt.Errorf("LLM max tokens must be 1,000,000 or less")
	}

	oldLLM := a.config.LLM

	// Switch provider first so we don't persist broken config
	if err := a.llmManager.SwitchProvider(&llmConfig); err != nil {
		return fmt.Errorf("failed to switch LLM provider: %w", err)
	}

	// Update config and persist
	a.config.LLM = llmConfig
	if err := a.configService.Save(a.config); err != nil {
		a.config.LLM = oldLLM
		_ = a.llmManager.SwitchProvider(&oldLLM)
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// GenerateTextStream generates text with streaming response
// Emits events: llm:stream:chunk, llm:stream:done, llm:stream:error
func (a *App) GenerateTextStream(prompt string, systemPrompt string) error {
	a.cancelActiveStream()

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}

	streamCtx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	streamID := a.setActiveStreamCancel(cancel)

	// Run streaming in goroutine to not block
	go func() {
		defer a.clearActiveStreamCancel(streamID)

		seenDone := false

		err := a.llmManager.GenerateStream(streamCtx, request,
			func(chunk *domain.StreamChunk) error {
				if chunk.Done {
					seenDone = true
					a.emitEvent("llm:stream:done", chunk)
				} else {
					a.emitEvent("llm:stream:chunk", chunk)
				}
				return nil
			})

		if err != nil {
			if errors.Is(err, context.Canceled) {
				if !seenDone {
					a.emitEvent("llm:stream:done", &domain.StreamChunk{
						Text:         "",
						Done:         true,
						FinishReason: "cancelled",
					})
				}
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				a.emitEvent("llm:stream:error", "stream timed out after 5 minutes")
				return
			}
			a.emitEvent("llm:stream:error", err.Error())
			return
		}

		if !seenDone {
			a.emitEvent("llm:stream:done", &domain.StreamChunk{
				Text:         "",
				Done:         true,
				FinishReason: "stop",
			})
		}
	}()

	return nil
}

// StopTextStream cancels the currently running text stream, if any.
func (a *App) StopTextStream() {
	a.cancelActiveStream()
}

func (a *App) setActiveStreamCancel(cancel context.CancelFunc) uint64 {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()
	a.streamID++
	a.streamCancel = cancel
	return a.streamID
}

func (a *App) clearActiveStreamCancel(streamID uint64) {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()
	if a.streamID == streamID {
		a.streamCancel = nil
	}
}

func (a *App) cancelActiveStream() {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()
	if a.streamCancel == nil {
		return
	}

	a.streamCancel()
	a.streamCancel = nil
}

// ExecutePromptOnNoteStream executes a prompt on a note with streaming
func (a *App) ExecutePromptOnNoteStream(promptID, noteID string) error {
	// Get the prompt
	prompt, err := a.promptService.Get(promptID)
	if err != nil {
		a.emitEvent("llm:stream:error", fmt.Sprintf("failed to get prompt: %v", err))
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	// Get the note
	note, err := a.noteService.Get(noteID)
	if err != nil {
		a.emitEvent("llm:stream:error", fmt.Sprintf("failed to get note: %v", err))
		return fmt.Errorf("failed to get note: %w", err)
	}

	// Replace {{content}} placeholder in user prompt
	userPrompt := strings.ReplaceAll(prompt.UserPrompt, "{{content}}", note.Content)

	return a.GenerateTextStream(userPrompt, prompt.SystemPrompt)
}

// ExecutePromptOnContentStream executes a prompt on content with streaming
func (a *App) ExecutePromptOnContentStream(promptID, content string) error {
	// Get the prompt
	prompt, err := a.promptService.Get(promptID)
	if err != nil {
		a.emitEvent("llm:stream:error", fmt.Sprintf("failed to get prompt: %v", err))
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	// Replace {{content}} placeholder in user prompt
	userPrompt := strings.ReplaceAll(prompt.UserPrompt, "{{content}}", content)

	return a.GenerateTextStream(userPrompt, prompt.SystemPrompt)
}

// GetStreamingSupport returns whether streaming is available
func (a *App) GetStreamingSupport() bool {
	supported := a.llmManager.SupportsStreaming()
	slog.Debug("GetStreamingSupport", "supported", supported, "llmAvailable", a.llmManager.IsAvailable())
	if provider := a.llmManager.GetProvider(); provider != nil {
		slog.Debug("GetStreamingSupport provider", "providerAvailable", provider.IsAvailable(), "providerSupportsStreaming", provider.SupportsStreaming())
	} else {
		slog.Debug("GetStreamingSupport: no provider configured")
	}
	return supported
}

// ============================================================================
// PROMPT OPERATIONS
// ============================================================================

// GetAllPrompts returns all available prompts
func (a *App) GetAllPrompts() ([]domain.Prompt, error) {
	return a.promptService.GetAll()
}

// GetPrompt returns a prompt by ID
func (a *App) GetPrompt(id string) (*domain.Prompt, error) {
	return a.promptService.Get(id)
}

// CreatePrompt creates a new prompt
func (a *App) CreatePrompt(name, description, systemPrompt, userPrompt string) (*domain.Prompt, error) {
	return a.promptService.Create(name, description, systemPrompt, userPrompt)
}

// UpdatePrompt updates an existing prompt
func (a *App) UpdatePrompt(id, name, description, systemPrompt, userPrompt string) error {
	return a.promptService.Update(id, name, description, systemPrompt, userPrompt)
}

// DeletePrompt deletes a prompt
func (a *App) DeletePrompt(id string) error {
	return a.promptService.Delete(id)
}

// ExecutePromptOnNote executes a prompt on a note's content
func (a *App) ExecutePromptOnNote(promptID, noteID string) (*domain.PromptExecutionResult, error) {
	// Get the prompt
	prompt, err := a.promptService.Get(promptID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt: %w", err)
	}

	// Get the note
	note, err := a.noteService.Get(noteID)
	if err != nil {
		return nil, fmt.Errorf("failed to get note: %w", err)
	}

	// Replace {{content}} placeholder in user prompt
	userPrompt := strings.ReplaceAll(prompt.UserPrompt, "{{content}}", note.Content)

	// Execute the prompt
	request := &domain.LLMRequest{
		Prompt:       userPrompt,
		SystemPrompt: prompt.SystemPrompt,
	}

	response, err := a.llmManager.Generate(a.ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	// Create execution result
	result := &domain.PromptExecutionResult{
		PromptName:  prompt.Name,
		Input:       note.Content,
		Output:      response.Text,
		TokensUsed:  response.TokensUsed,
		ExecutedAt:  time.Now(),
		LLMResponse: response,
	}

	return result, nil
}

// ExecutePromptOnContent executes a prompt on arbitrary content
func (a *App) ExecutePromptOnContent(promptID, content string) (*domain.PromptExecutionResult, error) {
	// Get the prompt
	prompt, err := a.promptService.Get(promptID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt: %w", err)
	}

	// Replace {{content}} placeholder in user prompt
	userPrompt := strings.ReplaceAll(prompt.UserPrompt, "{{content}}", content)

	// Execute the prompt
	request := &domain.LLMRequest{
		Prompt:       userPrompt,
		SystemPrompt: prompt.SystemPrompt,
	}

	response, err := a.llmManager.Generate(a.ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	// Create execution result
	result := &domain.PromptExecutionResult{
		PromptName:  prompt.Name,
		Input:       content,
		Output:      response.Text,
		TokensUsed:  response.TokensUsed,
		ExecutedAt:  time.Now(),
		LLMResponse: response,
	}

	return result, nil
}

// ============================================================================
// FOLDER OPERATIONS
// ============================================================================

func (a *App) GetAllFolders() ([]domain.Folder, error) {
	return a.folderService.GetAll()
}

func (a *App) CreateFolder(name string, parentID string) (*domain.Folder, error) {
	return a.folderService.Create(name, parentID)
}

func (a *App) UpdateFolder(id string, name string, parentID string) error {
	return a.folderService.Update(id, name, parentID)
}

func (a *App) DeleteFolder(id string, deleteNotes bool) error {
	return a.folderService.Delete(id, deleteNotes, a.noteService)
}

func (a *App) GetFolderPath(folderID string) ([]domain.Folder, error) {
	return a.folderService.GetPath(folderID)
}

// ============================================================================
// NOTE OPERATIONS
// ============================================================================

func (a *App) GetAllNotes() ([]domain.Note, error) {
	return a.noteService.GetAll()
}

func (a *App) GetNote(id string) (*domain.Note, error) {
	return a.noteService.Get(id)
}

func (a *App) CreateNote(title string, content string, folderID string) (*domain.Note, error) {
	return a.noteService.Create(title, content, folderID)
}

func (a *App) UpdateNote(id string, title string, content string) error {
	return a.noteService.Update(id, title, content)
}

func (a *App) MoveNote(noteID string, targetFolderID string) error {
	return a.noteService.Move(noteID, targetFolderID)
}

func (a *App) DeleteNote(id string) error {
	return a.noteService.Delete(id)
}

func (a *App) SearchNotes(query string, limit int) ([]domain.SearchMatch, error) {
	return a.noteService.Search(query, limit)
}

// ============================================================================
// CONFIG OPERATIONS
// ============================================================================

// GetConfig returns the current application configuration
func (a *App) GetConfig() *domain.Config {
	return a.config
}

// IsFirstRun returns true when config.json was created on this launch.
func (a *App) IsFirstRun() bool {
	if a.configService == nil {
		return false
	}
	return a.configService.IsFirstRun()
}

// SaveConfig saves the configuration and reinitializes affected services
func (a *App) SaveConfig(config domain.Config) error {
	// Validate configuration
	if config.ModelName == "" {
		return fmt.Errorf("STT model name cannot be empty")
	}
	if strings.TrimSpace(config.STTLanguage) == "" {
		return fmt.Errorf("STT language cannot be empty")
	}
	if config.LLM.Temperature < 0 || config.LLM.Temperature > 2 {
		return fmt.Errorf("LLM temperature must be between 0 and 2")
	}
	if config.LLM.MaxTokens < 50 {
		return fmt.Errorf("LLM max tokens must be more than 50")
	}
	if config.LLM.MaxTokens > 1_000_000 {
		return fmt.Errorf("LLM max tokens must be 1,000,000 or less")
	}
	if config.Audio.Mixer.MicrophoneGain < 0 || config.Audio.Mixer.MicrophoneGain > 2 {
		return fmt.Errorf("microphone gain must be between 0 and 2")
	}
	if config.Audio.Mixer.SystemGain < 0 || config.Audio.Mixer.SystemGain > 2 {
		return fmt.Errorf("system gain must be between 0 and 2")
	}

	oldConfig := a.config

	// Reinitialize STT if model changed or service is not initialized yet.
	if policy.ShouldInitializeSTTOnSave(oldConfig.ModelName, config.ModelName, oldConfig.STTLanguage, config.STTLanguage, a.sttManager.IsAvailable()) {
		sttConfig := &domain.STTConfig{
			ModelName: config.ModelName,
			Language:  config.STTLanguage,
		}
		if err := a.sttManager.Initialize(sttConfig); err != nil {
			slog.Warn("STT reinitialization failed", "error", err)
		}
	}

	// Reinitialize LLM if config changed or service is not initialized yet.
	if policy.ShouldInitializeLLMOnSave(oldConfig.LLM, config.LLM, a.llmManager.IsAvailable()) {
		if err := a.llmManager.SwitchProvider(&config.LLM); err != nil {
			slog.Warn("LLM reinitialization failed", "error", err)
			return fmt.Errorf("failed to reinitialize LLM provider: %w", err)
		}
	}

	// Save configuration to file
	if err := a.configService.Save(&config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Update in-memory config
	a.config = &config

	// Update audio settings
	if oldConfig.Audio.DefaultSource != config.Audio.DefaultSource {
		audioSource := domain.AudioSourceFromString(config.Audio.DefaultSource)
		if err := a.sttManager.SetAudioSource(audioSource); err != nil {
			slog.Warn("Failed to set audio source", "error", err)
		}
	}

	// Update mixer config
	if a.audioManager != nil {
		a.audioManager.SetMixerConfig(config.Audio.Mixer)
	}

	slog.Info("Configuration saved and services reinitialized successfully")
	a.emitEvent("config:saved")
	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (a *App) loadStructure() (*domain.FolderStructure, error) {
	return a.structureRepo.Load()
}
