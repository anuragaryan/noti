package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/service"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx           context.Context
	basePath      string
	structurePath string
	notesPath     string
	config        *domain.Config
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	fileSystem    *repository.FileSystem
	folderService *service.FolderService
	noteService   *service.NoteService
	configService *service.ConfigService
	sttManager    *service.STTManager
	llmManager    *service.LLMManager
	promptService *service.PromptService
	audioManager  *service.AudioManager
}

// NewApp creates a new App application struct
func NewApp() *App {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("could not determine user config directory: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("could not determine user config directory: %v", err)
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
	sttManager := service.NewSTTManager(basePath)
	llmManager := service.NewLLMManager(basePath)
	promptService := service.NewPromptService(basePath)
	audioManager := service.NewAudioManager()

	// Connect audio manager to STT manager
	sttManager.SetAudioManager(audioManager)

	return &App{
		basePath:      basePath,
		structurePath: structurePath,
		notesPath:     notesPath,
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		fileSystem:    fileSystem,
		folderService: folderService,
		noteService:   noteService,
		configService: configService,
		sttManager:    sttManager,
		llmManager:    llmManager,
		promptService: promptService,
		audioManager:  audioManager,
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Create notes directory if it doesn't exist
	if err := os.MkdirAll(a.notesPath, 0755); err != nil {
		fmt.Printf("ERROR: Cannot create notes directory: %v\n", err)
		fmt.Printf("Path: %s\n", a.notesPath)
		fmt.Println("Please check that the application has permission to write to the Documents folder.")
		return
	}

	// Test write permissions by creating a test file
	testFile := filepath.Join(a.notesPath, ".permission_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		fmt.Printf("ERROR: Cannot write to notes directory: %v\n", err)
		fmt.Printf("Path: %s\n", a.notesPath)
		fmt.Println("The application does not have write permissions.")
		return
	}
	os.Remove(testFile)
	fmt.Printf("Successfully initialized notes directory at: %s\n", a.notesPath)

	// Load config
	config, err := a.configService.Load()
	if err != nil {
		fmt.Printf("ERROR: Cannot load config: %v\n", err)
		// Use a default config if loading fails
		a.config = &domain.Config{RealtimeTranscriptionChunkSeconds: 3, ModelName: "base.en"}
		fmt.Println("Using default STT config.")
	} else {
		a.config = config
	}

	// Initialize Audio Manager
	a.audioManager.SetContext(ctx)
	if err := a.audioManager.Initialize(); err != nil {
		fmt.Printf("Audio Manager initialization failed: %v\n", err)
		fmt.Println("Audio capture features may be limited.")
	}

	// Apply audio source from config after audio manager is initialized
	audioSource := domain.AudioSourceFromString(a.config.Audio.DefaultSource)
	if err := a.audioManager.SetAudioSource(audioSource); err != nil {
		fmt.Printf("Warning: Failed to set audio source from config: %v\n", err)
	} else {
		a.sttManager.SetAudioSource(audioSource)
		fmt.Printf("Audio source set to: %s\n", a.config.Audio.DefaultSource)
	}

	// Initialize STT service with self-healing
	a.sttManager.SetContext(ctx)
	sttConfig := &domain.STTConfig{
		ModelName:         a.config.ModelName,
		ChunkDurationSecs: a.config.RealtimeTranscriptionChunkSeconds,
	}
	if err := a.sttManager.Initialize(sttConfig); err != nil {
		fmt.Printf("STT initialization failed: %v\n", err)
	}

	// Initialize LLM service if configured
	a.llmManager.SetContext(ctx)
	if a.config.LLM.Provider != "" {
		if err := a.llmManager.Initialize(&a.config.LLM); err != nil {
			fmt.Printf("LLM initialization failed: %v\n", err)
			fmt.Println("LLM features will be disabled. You can configure LLM settings later.")
		}
	} else {
		fmt.Println("LLM not configured. You can enable it in settings.")
	}

	// Initialize prompt service
	if err := a.promptService.Initialize(); err != nil {
		fmt.Printf("Prompt service initialization failed: %v\n", err)
	}

	// Create structure.json if it doesn't exist
	if _, err := os.Stat(a.structurePath); os.IsNotExist(err) {
		if err := a.structureRepo.Save(&domain.FolderStructure{
			Folders: []domain.Folder{},
			Notes:   []domain.Note{},
		}); err != nil {
			fmt.Printf("ERROR: Cannot create structure.json: %v\n", err)
			return
		}
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
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
	fmt.Printf("[App.GenerateText] Called with prompt length: %d\n", len(prompt))
	fmt.Printf("[App.GenerateText] LLM Manager available: %v\n", a.llmManager.IsAvailable())

	if !a.llmManager.IsAvailable() {
		fmt.Println("[App.GenerateText] LLM service not available")
		status := a.llmManager.GetStatus()
		fmt.Printf("[App.GenerateText] Status: %+v\n", status)
		return nil, fmt.Errorf("LLM service not available. Please configure LLM settings")
	}

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}

	fmt.Println("[App.GenerateText] Calling llmManager.Generate...")
	return a.llmManager.Generate(a.ctx, request)
}

// GenerateTextWithOptions generates text with custom parameters
func (a *App) GenerateTextWithOptions(prompt string, systemPrompt string, temperature float32, maxTokens int) (*domain.LLMResponse, error) {
	if !a.llmManager.IsAvailable() {
		return nil, fmt.Errorf("LLM service not available. Please configure LLM settings")
	}

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		Temperature:  temperature,
		MaxTokens:    maxTokens,
	}

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

	fmt.Printf("[DEBUG] LLM Status: %+v\n", status)
	return status
}

// UpdateLLMConfig updates LLM configuration and switches provider
func (a *App) UpdateLLMConfig(llmConfig domain.LLMConfig) error {
	// Update config
	a.config.LLM = llmConfig

	// Save config
	if err := a.configService.Save(a.config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Switch provider
	if err := a.llmManager.SwitchProvider(&llmConfig); err != nil {
		return fmt.Errorf("failed to switch LLM provider: %w", err)
	}

	return nil
}

// GenerateTextStream generates text with streaming response
// Emits events: llm:stream:chunk, llm:stream:done, llm:stream:error
func (a *App) GenerateTextStream(prompt string, systemPrompt string) error {
	if !a.llmManager.SupportsStreaming() {
		runtime.EventsEmit(a.ctx, "llm:stream:error", "Streaming not available")
		return fmt.Errorf("streaming not available")
	}

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}

	// Run streaming in goroutine to not block
	go func() {
		err := a.llmManager.GenerateStream(a.ctx, request,
			func(chunk *domain.StreamChunk) error {
				if chunk.Done {
					runtime.EventsEmit(a.ctx, "llm:stream:done", chunk)
				} else {
					runtime.EventsEmit(a.ctx, "llm:stream:chunk", chunk)
				}
				return nil
			})

		if err != nil {
			runtime.EventsEmit(a.ctx, "llm:stream:error", err.Error())
		}
	}()

	return nil
}

// GenerateTextStreamWithOptions generates text with streaming and custom parameters
func (a *App) GenerateTextStreamWithOptions(prompt string, systemPrompt string, temperature float32, maxTokens int) error {
	if !a.llmManager.SupportsStreaming() {
		runtime.EventsEmit(a.ctx, "llm:stream:error", "Streaming not available")
		return fmt.Errorf("streaming not available")
	}

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
		Temperature:  temperature,
		MaxTokens:    maxTokens,
	}

	// Run streaming in goroutine to not block
	go func() {
		err := a.llmManager.GenerateStream(a.ctx, request,
			func(chunk *domain.StreamChunk) error {
				if chunk.Done {
					runtime.EventsEmit(a.ctx, "llm:stream:done", chunk)
				} else {
					runtime.EventsEmit(a.ctx, "llm:stream:chunk", chunk)
				}
				return nil
			})

		if err != nil {
			runtime.EventsEmit(a.ctx, "llm:stream:error", err.Error())
		}
	}()

	return nil
}

// ExecutePromptOnNoteStream executes a prompt on a note with streaming
func (a *App) ExecutePromptOnNoteStream(promptID, noteID string) error {
	// Get the prompt
	prompt, err := a.promptService.Get(promptID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "llm:stream:error", fmt.Sprintf("failed to get prompt: %v", err))
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	// Get the note
	note, err := a.noteService.Get(noteID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "llm:stream:error", fmt.Sprintf("failed to get note: %v", err))
		return fmt.Errorf("failed to get note: %w", err)
	}

	// Replace {{content}} placeholder in user prompt
	userPrompt := strings.ReplaceAll(prompt.UserPrompt, "{{content}}", note.Content)

	return a.GenerateTextStreamWithOptions(userPrompt, prompt.SystemPrompt, prompt.Temperature, prompt.MaxTokens)
}

// ExecutePromptOnContentStream executes a prompt on content with streaming
func (a *App) ExecutePromptOnContentStream(promptID, content string) error {
	// Get the prompt
	prompt, err := a.promptService.Get(promptID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "llm:stream:error", fmt.Sprintf("failed to get prompt: %v", err))
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	// Replace {{content}} placeholder in user prompt
	userPrompt := strings.ReplaceAll(prompt.UserPrompt, "{{content}}", content)

	return a.GenerateTextStreamWithOptions(userPrompt, prompt.SystemPrompt, prompt.Temperature, prompt.MaxTokens)
}

// GetStreamingSupport returns whether streaming is available
func (a *App) GetStreamingSupport() bool {
	supported := a.llmManager.SupportsStreaming()
	fmt.Printf("[App.GetStreamingSupport] Streaming supported: %v\n", supported)
	fmt.Printf("[App.GetStreamingSupport] LLM available: %v\n", a.llmManager.IsAvailable())
	if provider := a.llmManager.GetProvider(); provider != nil {
		fmt.Printf("[App.GetStreamingSupport] Provider available: %v\n", provider.IsAvailable())
		fmt.Printf("[App.GetStreamingSupport] Provider supports streaming: %v\n", provider.SupportsStreaming())
	} else {
		fmt.Printf("[App.GetStreamingSupport] No provider configured\n")
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
func (a *App) CreatePrompt(name, description, systemPrompt, userPrompt string, temperature float32, maxTokens int) (*domain.Prompt, error) {
	return a.promptService.Create(name, description, systemPrompt, userPrompt, temperature, maxTokens)
}

// UpdatePrompt updates an existing prompt
func (a *App) UpdatePrompt(id, name, description, systemPrompt, userPrompt string, temperature float32, maxTokens int) error {
	return a.promptService.Update(id, name, description, systemPrompt, userPrompt, temperature, maxTokens)
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
		Temperature:  prompt.Temperature,
		MaxTokens:    prompt.MaxTokens,
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
		Temperature:  prompt.Temperature,
		MaxTokens:    prompt.MaxTokens,
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

// ============================================================================
// CONFIG OPERATIONS
// ============================================================================

// GetConfig returns the current application configuration
func (a *App) GetConfig() *domain.Config {
	return a.config
}

// SaveConfig saves the configuration and reinitializes affected services
func (a *App) SaveConfig(config domain.Config) error {
	// Validate configuration
	if config.RealtimeTranscriptionChunkSeconds < 1 || config.RealtimeTranscriptionChunkSeconds > 30 {
		return fmt.Errorf("transcription chunk seconds must be between 1 and 30")
	}
	if config.ModelName == "" {
		return fmt.Errorf("STT model name cannot be empty")
	}
	if config.LLM.Temperature < 0 || config.LLM.Temperature > 2 {
		return fmt.Errorf("LLM temperature must be between 0 and 2")
	}
	if config.LLM.MaxTokens < 50 {
		return fmt.Errorf("LLM max tokens must be more than 50")
	}
	if config.Audio.SampleRate != 8000 && config.Audio.SampleRate != 16000 &&
		config.Audio.SampleRate != 22050 && config.Audio.SampleRate != 44100 &&
		config.Audio.SampleRate != 48000 {
		return fmt.Errorf("invalid sample rate")
	}
	if config.Audio.Mixer.MicrophoneGain < 0 || config.Audio.Mixer.MicrophoneGain > 2 {
		return fmt.Errorf("microphone gain must be between 0 and 2")
	}
	if config.Audio.Mixer.SystemGain < 0 || config.Audio.Mixer.SystemGain > 2 {
		return fmt.Errorf("system gain must be between 0 and 2")
	}

	// Save configuration to file
	if err := a.configService.Save(&config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Update in-memory config
	oldConfig := a.config
	a.config = &config

	// Reinitialize STT if model changed
	if oldConfig.ModelName != config.ModelName ||
		oldConfig.RealtimeTranscriptionChunkSeconds != config.RealtimeTranscriptionChunkSeconds {
		sttConfig := &domain.STTConfig{
			ModelName:         config.ModelName,
			ChunkDurationSecs: config.RealtimeTranscriptionChunkSeconds,
		}
		if err := a.sttManager.Initialize(sttConfig); err != nil {
			fmt.Printf("Warning: STT reinitialization failed: %v\n", err)
		}
	}

	// Reinitialize LLM if config changed
	if oldConfig.LLM.Provider != config.LLM.Provider ||
		oldConfig.LLM.ModelName != config.LLM.ModelName ||
		oldConfig.LLM.APIEndpoint != config.LLM.APIEndpoint ||
		oldConfig.LLM.APIKey != config.LLM.APIKey {
		if err := a.llmManager.SwitchProvider(&config.LLM); err != nil {
			fmt.Printf("Warning: LLM reinitialization failed: %v\n", err)
		}
	}

	// Update audio settings
	if oldConfig.Audio.DefaultSource != config.Audio.DefaultSource {
		audioSource := domain.AudioSourceFromString(config.Audio.DefaultSource)
		if err := a.sttManager.SetAudioSource(audioSource); err != nil {
			fmt.Printf("Warning: Failed to set audio source: %v\n", err)
		}
	}

	// Update mixer config
	if a.audioManager != nil {
		a.audioManager.SetMixerConfig(config.Audio.Mixer)
	}

	fmt.Println("Configuration saved and services reinitialized successfully")
	runtime.EventsEmit(a.ctx, "config:saved")
	return nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (a *App) loadStructure() (*domain.FolderStructure, error) {
	return a.structureRepo.Load()
}
