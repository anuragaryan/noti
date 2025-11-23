package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/service"
)

// App struct
type App struct {
	ctx           context.Context
	basePath      string
	configPath    string
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
}

// NewApp creates a new App application struct
func NewApp() *App {
	homeDir, _ := os.UserHomeDir()
	basePath := filepath.Join(homeDir, "Documents", "Noti")
	notesPath := filepath.Join(basePath, "notes")
	configPath := filepath.Join(notesPath, "structure.json")

	// Initialize repositories
	structureRepo := repository.NewStructureRepository(configPath)
	pathResolver := repository.NewPathResolver(notesPath)
	fileSystem := repository.NewFileSystem(pathResolver)

	// Initialize services
	folderService := service.NewFolderService(structureRepo, pathResolver, notesPath)
	noteService := service.NewNoteService(structureRepo, pathResolver, fileSystem, notesPath)
	configService := service.NewConfigService(basePath, defaultConfig)
	sttManager := service.NewSTTManager(basePath, downloadScript)
	llmManager := service.NewLLMManager(basePath, downloadScriptLLM)
	promptService := service.NewPromptService(basePath)

	return &App{
		basePath:      basePath,
		configPath:    configPath,
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

	// Initialize STT service with self-healing
	a.sttManager.SetContext(ctx)
	if err := a.sttManager.Initialize(a.config.RealtimeTranscriptionChunkSeconds, a.config.ModelName, NewSTTServiceAdapter); err != nil {
		fmt.Printf("STT initialization failed: %v\n", err)
	}

	// Initialize LLM service if configured
	a.llmManager.SetContext(ctx)
	if a.config.LLM.Provider != "" {
		if err := a.llmManager.Initialize(&a.config.LLM, NewLLMProvider); err != nil {
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
	if _, err := os.Stat(a.configPath); os.IsNotExist(err) {
		if err := a.saveStructure(&domain.FolderStructure{
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
}

// ============================================================================
// STT OPERATIONS
// ============================================================================

// StartVoiceRecording starts recording audio from microphone
func (a *App) StartVoiceRecording() error {
	if !a.sttManager.IsAvailable() {
		return fmt.Errorf("STT service not available. Please download the Whisper model")
	}
	return a.sttManager.GetService().StartRecording()
}

// StopVoiceRecording stops recording and returns transcribed text
func (a *App) StopVoiceRecording() (*TranscriptionResult, error) {
	if !a.sttManager.IsAvailable() {
		return nil, fmt.Errorf("STT service not available")
	}
	result, err := a.sttManager.GetService().StopRecording()
	if err != nil {
		return nil, err
	}
	return result.(*TranscriptionResult), nil
}

// IsRecording returns current recording status
func (a *App) IsRecording() bool {
	if !a.sttManager.IsAvailable() {
		return false
	}
	return a.sttManager.GetService().IsRecording()
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
// LLM OPERATIONS
// ============================================================================

// GenerateText generates text using the configured LLM
func (a *App) GenerateText(prompt string, systemPrompt string) (*domain.LLMResponse, error) {
	if !a.llmManager.IsAvailable() {
		return nil, fmt.Errorf("LLM service not available. Please configure LLM settings")
	}

	request := &domain.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}

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
	return a.llmManager.GetStatus()
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
	if err := a.llmManager.SwitchProvider(&llmConfig, NewLLMProvider); err != nil {
		return fmt.Errorf("failed to switch LLM provider: %w", err)
	}

	return nil
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

// DownloadLLMModel downloads a local LLM model
func (a *App) DownloadLLMModel(modelName string) error {
	return a.llmManager.DownloadModel(modelName, NewLLMProvider)
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
// HELPER FUNCTIONS
// ============================================================================

func (a *App) loadStructure() (*domain.FolderStructure, error) {
	return a.structureRepo.Load()
}

func (a *App) saveStructure(structure *domain.FolderStructure) error {
	return a.structureRepo.Save(structure)
}
