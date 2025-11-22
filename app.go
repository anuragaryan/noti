package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/service"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx           context.Context
	basePath      string
	configPath    string
	notesPath     string
	sttService    *STTService
	config        *domain.Config
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	fileSystem    *repository.FileSystem
	folderService *service.FolderService
	noteService   *service.NoteService
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

	return &App{
		basePath:      basePath,
		configPath:    configPath,
		notesPath:     notesPath,
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		fileSystem:    fileSystem,
		folderService: folderService,
		noteService:   noteService,
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
	if err := a.loadConfig(); err != nil {
		fmt.Printf("ERROR: Cannot load config: %v\n", err)
		// Use a default config if loading fails
		a.config = &domain.Config{RealtimeTranscriptionChunkSeconds: 3}
		fmt.Println("Using default STT config.")
	}

	// --- STT Service Initialization with Self-Healing ---

	// Helper function to attempt STT initialization
	tryInitializeSTT := func() bool {
		sttService, err := NewSTTService(a.basePath, a.config.RealtimeTranscriptionChunkSeconds, a.config.ModelName)
		if err != nil {
			// This handles file-not-found from NewSTTService
			return false
		}
		if err := sttService.Initialize(); err != nil {
			// This handles model loading/corruption errors (like the tensor error)
			fmt.Printf("Failed to initialize STT model: %v\n", err)
			return false
		}
		// Success
		sttService.SetContext(ctx)
		a.sttService = sttService
		fmt.Println("STT service initialized successfully.")
		runtime.EventsEmit(a.ctx, "stt:ready")
		return true
	}

	if !tryInitializeSTT() {
		// Initialization failed, likely because the model is missing or corrupt.
		fmt.Println("STT initialization failed. Attempting to download or re-download model...")

		// Delete the potentially corrupt model file before downloading.
		modelFileName := fmt.Sprintf("ggml-%s.bin", a.config.ModelName)
		modelPath := filepath.Join(a.basePath, "models", modelFileName)
		if _, err := os.Stat(modelPath); err == nil {
			fmt.Printf("Deleting existing model file at %s to ensure a clean download.\n", modelPath)
			os.Remove(modelPath)
		}

		// downloadModel will download and then try to initialize again internally.
		if err := a.downloadModel(a.config.ModelName); err != nil {
			fmt.Printf("ERROR: Model download and initialization failed: %v\n", err)
			fmt.Println("Speech-to-text features will be disabled.")
		}
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
	if a.sttService != nil {
		a.sttService.Cleanup()
	}
}

// ============================================================================
// STT OPERATIONS
// ============================================================================

// StartVoiceRecording starts recording audio from microphone
func (a *App) StartVoiceRecording() error {
	if a.sttService == nil {
		return fmt.Errorf("STT service not available. Please download the Whisper model")
	}
	return a.sttService.StartRecording()
}

// StopVoiceRecording stops recording and returns transcribed text
func (a *App) StopVoiceRecording() (*TranscriptionResult, error) {
	if a.sttService == nil {
		return nil, fmt.Errorf("STT service not available")
	}
	return a.sttService.StopRecording()
}

// IsRecording returns current recording status
func (a *App) IsRecording() bool {
	if a.sttService == nil {
		return false
	}
	return a.sttService.IsRecording()
}

// GetSTTStatus returns whether STT is available
func (a *App) GetSTTStatus() map[string]interface{} {
	available := a.sttService != nil
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

// loadConfig loads the application configuration from config.json
func (a *App) loadConfig() error {
	configFilePath := filepath.Join(a.basePath, "config.json")

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		// Config file does not exist, so create it from the embedded template
		fmt.Printf("config.json not found. Creating from embedded template at %s\n", configFilePath)
		// Ensure the directory for config.json exists.
		dir := filepath.Dir(configFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for config.json: %w", err)
		}
		if err := os.WriteFile(configFilePath, defaultConfig, 0644); err != nil {
			return fmt.Errorf("failed to write default config file: %w", err)
		}
		// Use the embedded config data for this session
		data = defaultConfig
	}

	// Unmarshal the config data (either from file or the embedded default)
	var config domain.Config
	if err := json.Unmarshal(data, &config); err != nil {
		// If unmarshalling fails, it could be a corrupt file. Try to restore it.
		fmt.Printf("WARNING: Failed to unmarshal config.json: %v. Restoring from template.\n", err)
		if err := os.WriteFile(configFilePath, defaultConfig, 0644); err != nil {
			return fmt.Errorf("failed to restore default config file: %w", err)
		}
		data = defaultConfig // Use the default config for this session
		// Retry unmarshalling with the default data
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to unmarshal embedded default config: %w", err)
		}
	}

	// Set defaults for any fields that might be missing (for backward compatibility)
	if config.RealtimeTranscriptionChunkSeconds <= 0 {
		config.RealtimeTranscriptionChunkSeconds = 3
	}
	if config.ModelName == "" {
		config.ModelName = "base.en"
	}

	a.config = &config
	fmt.Printf("Loaded config from: %s\n", configFilePath)
	return nil
}

// downloadModel runs the embedded script to download a model
func (a *App) downloadModel(modelName string) error {
	runtime.EventsEmit(a.ctx, "download:start", modelName)

	// Get the models directory
	modelsPath := filepath.Join(a.basePath, "models")
	if err := os.MkdirAll(modelsPath, 0755); err != nil {
		runtime.EventsEmit(a.ctx, "download:error", "Failed to create models directory")
		return fmt.Errorf("failed to create models directory: %w", err)
	}

	// Define the script path inside the models directory
	scriptPath := filepath.Join(modelsPath, ".download-ggml-model.sh")

	// Write the embedded script to the destination and make it executable
	if err := os.WriteFile(scriptPath, downloadScript, 0755); err != nil {
		runtime.EventsEmit(a.ctx, "download:error", "Failed to write download script")
		return fmt.Errorf("failed to write download script: %w", err)
	}
	// Ensure the script is cleaned up
	defer os.Remove(scriptPath)

	// Run the script from within the models directory
	cmd := exec.Command(scriptPath, modelName)
	cmd.Dir = modelsPath // Set the working directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Model download script failed:\n%s\n", string(output))
		runtime.EventsEmit(a.ctx, "download:error", "Model download failed. Check logs.")
		return fmt.Errorf("model download script failed: %w", err)
	}

	fmt.Printf("Model download script output:\n%s\n", string(output))
	runtime.EventsEmit(a.ctx, "download:finish", modelName)

	// After a successful download, re-initialize the STT service
	fmt.Println("Re-initializing STT service after model download...")
	sttService, err := NewSTTService(a.basePath, a.config.RealtimeTranscriptionChunkSeconds, a.config.ModelName)
	if err != nil {
		fmt.Printf("Warning: STT service initialization failed after download: %v\n", err)
	} else {
		if err := sttService.Initialize(); err != nil {
			fmt.Printf("Warning: Failed to load STT model after download: %v\n", err)
		} else {
			sttService.SetContext(a.ctx)
			a.sttService = sttService
			fmt.Println("STT service initialized successfully after download.")
			// Notify the frontend that the service is now ready
			runtime.EventsEmit(a.ctx, "stt:ready")
		}
	}

	return nil
}
