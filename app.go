package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
