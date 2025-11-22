package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	configPath string
	notesPath  string
	sttService *STTService
	config     *Config
}

// Folder represents a folder/category
type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  string    `json:"parentId"`
	CreatedAt time.Time `json:"createdAt"`
	Order     int       `json:"order"`
}

// Note represents a note entry
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	FolderID  string    `json:"folderId"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Order     int       `json:"order"`
}

// FolderStructure represents the folder/note organization
type FolderStructure struct {
	Folders []Folder `json:"folders"`
	Notes   []Note   `json:"notes"`
}

// Config represents the application configuration
type Config struct {
	RealtimeTranscriptionChunkSeconds int    `json:"realtimeTranscriptionChunkSeconds"`
	ModelName                         string `json:"modelName"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	homeDir, _ := os.UserHomeDir()
	// Use Documents folder instead of Application Support for better compatibility
	// Documents folder is always accessible without special entitlements in production builds
	appSupport := filepath.Join(homeDir, "Documents", "Noti")

	return &App{
		configPath: filepath.Join(appSupport, "structure.json"),
		notesPath:  appSupport,
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
		a.config = &Config{RealtimeTranscriptionChunkSeconds: 3}
		fmt.Println("Using default STT config.")
	}

	// --- STT Service Initialization with Self-Healing ---

	// Helper function to attempt STT initialization
	tryInitializeSTT := func() bool {
		sttService, err := NewSTTService(a.notesPath, a.config.RealtimeTranscriptionChunkSeconds, a.config.ModelName)
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
		modelPath := filepath.Join(a.notesPath, "models", modelFileName)
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
		if err := a.saveStructure(&FolderStructure{
			Folders: []Folder{},
			Notes:   []Note{},
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
	modelPath := filepath.Join(a.notesPath, "models", modelFileName)

	return map[string]interface{}{
		"available": available,
		"modelPath": modelPath,
	}
}

// ============================================================================
// FOLDER OPERATIONS (unchanged from previous implementation)
// ============================================================================

func (a *App) GetAllFolders() ([]Folder, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}
	return structure.Folders, nil
}

func (a *App) GetFolder(id string) (*Folder, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}

	for _, folder := range structure.Folders {
		if folder.ID == id {
			return &folder, nil
		}
	}

	return nil, fmt.Errorf("folder not found")
}

func (a *App) CreateFolder(name string, parentID string) (*Folder, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %v", err)
	}

	if parentID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == parentID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("parent folder not found")
		}
	}

	now := time.Now()
	folder := Folder{
		ID:        fmt.Sprintf("f_%d", now.UnixNano()),
		Name:      name,
		ParentID:  parentID,
		CreatedAt: now,
		Order:     len(structure.Folders),
	}

	if err := a.ensureFolderExists(folder.ID); err != nil {
		return nil, fmt.Errorf("failed to create folder directory: %v", err)
	}

	structure.Folders = append(structure.Folders, folder)
	if err := a.saveStructure(structure); err != nil {
		return nil, fmt.Errorf("failed to save structure: %v", err)
	}

	return &folder, nil
}

func (a *App) UpdateFolder(id string, name string, parentID string) error {
	structure, err := a.loadStructure()
	if err != nil {
		return err
	}

	if parentID != "" && parentID != id {
		if err := a.validateFolderMove(id, parentID, structure); err != nil {
			return err
		}
	}

	for i, folder := range structure.Folders {
		if folder.ID == id {
			structure.Folders[i].Name = name
			if parentID != folder.ParentID {
				structure.Folders[i].ParentID = parentID
			}
			return a.saveStructure(structure)
		}
	}

	return fmt.Errorf("folder not found")
}

func (a *App) DeleteFolder(id string, deleteNotes bool) error {
	structure, err := a.loadStructure()
	if err != nil {
		return err
	}

	for _, folder := range structure.Folders {
		if folder.ParentID == id {
			return fmt.Errorf("cannot delete folder with subfolders")
		}
	}

	if deleteNotes {
		notesToDelete := []string{}
		for _, note := range structure.Notes {
			if note.FolderID == id {
				notesToDelete = append(notesToDelete, note.ID)
			}
		}
		for _, noteID := range notesToDelete {
			if err := a.deleteNoteFile(noteID, id); err != nil {
				fmt.Printf("Warning: failed to delete note file %s: %v\n", noteID, err)
			}
		}

		newNotes := []Note{}
		for _, note := range structure.Notes {
			if note.FolderID != id {
				newNotes = append(newNotes, note)
			}
		}
		structure.Notes = newNotes
	} else {
		for i, note := range structure.Notes {
			if note.FolderID == id {
				if err := a.moveNoteFile(note.ID, id, ""); err != nil {
					return err
				}
				structure.Notes[i].FolderID = ""
			}
		}
	}

	newFolders := []Folder{}
	for _, folder := range structure.Folders {
		if folder.ID != id {
			newFolders = append(newFolders, folder)
		}
	}
	structure.Folders = newFolders

	folderPath := a.getFolderPath(id)
	os.RemoveAll(folderPath)

	return a.saveStructure(structure)
}

func (a *App) GetFolderPath(folderID string) ([]Folder, error) {
	if folderID == "" {
		return []Folder{}, nil
	}

	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}

	path := []Folder{}
	currentID := folderID

	for currentID != "" {
		found := false
		for _, folder := range structure.Folders {
			if folder.ID == currentID {
				path = append([]Folder{folder}, path...)
				currentID = folder.ParentID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	return path, nil
}

func (a *App) GetSubfolders(parentID string) ([]Folder, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}

	subfolders := []Folder{}
	for _, folder := range structure.Folders {
		if folder.ParentID == parentID {
			subfolders = append(subfolders, folder)
		}
	}

	return subfolders, nil
}

// ============================================================================
// NOTE OPERATIONS (unchanged from previous implementation)
// ============================================================================

func (a *App) GetAllNotes() ([]Note, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}
	return structure.Notes, nil
}

func (a *App) GetNote(id string) (*Note, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}

	for _, note := range structure.Notes {
		if note.ID == id {
			content, err := a.loadNoteContent(id, note.FolderID)
			if err != nil {
				return nil, err
			}
			note.Content = content
			return &note, nil
		}
	}

	return nil, fmt.Errorf("note not found")
}

func (a *App) GetNotesByFolder(folderID string) ([]Note, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, err
	}

	notes := []Note{}
	for _, note := range structure.Notes {
		if note.FolderID == folderID {
			notes = append(notes, note)
		}
	}

	return notes, nil
}

func (a *App) CreateNote(title string, content string, folderID string) (*Note, error) {
	structure, err := a.loadStructure()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %v", err)
	}

	if folderID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == folderID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("folder not found")
		}
	}

	now := time.Now()
	note := Note{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		Title:     title,
		FolderID:  folderID,
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
		Order:     len(structure.Notes),
	}

	if folderID != "" {
		if err := a.ensureFolderExists(folderID); err != nil {
			return nil, fmt.Errorf("failed to create folder directory: %v", err)
		}
	}

	notePath := a.getFolderPath(folderID)
	if folderID == "" {
		notePath = filepath.Join(a.notesPath, fmt.Sprintf("%s.md", note.ID))
	} else {
		notePath = filepath.Join(notePath, fmt.Sprintf("%s.md", note.ID))
	}
	if err := a.saveNoteContent(note.ID, folderID, content); err != nil {
		return nil, fmt.Errorf("failed to save note content: %v", err)
	}

	structure.Notes = append(structure.Notes, note)
	if err := a.saveStructure(structure); err != nil {
		return nil, fmt.Errorf("failed to save structure: %v", err)
	}

	return &note, nil
}

func (a *App) UpdateNote(id string, title string, content string) error {
	structure, err := a.loadStructure()
	if err != nil {
		return err
	}

	for i, note := range structure.Notes {
		if note.ID == id {
			structure.Notes[i].Title = title
			structure.Notes[i].UpdatedAt = time.Now()

			if err := a.saveNoteContent(id, note.FolderID, content); err != nil {
				return err
			}

			return a.saveStructure(structure)
		}
	}

	return fmt.Errorf("note not found")
}

func (a *App) MoveNote(noteID string, targetFolderID string) error {
	structure, err := a.loadStructure()
	if err != nil {
		return err
	}

	if targetFolderID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == targetFolderID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("target folder not found")
		}
	}

	for i, note := range structure.Notes {
		if note.ID == noteID {
			oldFolderID := note.FolderID

			if err := a.moveNoteFile(noteID, oldFolderID, targetFolderID); err != nil {
				return err
			}

			structure.Notes[i].FolderID = targetFolderID
			structure.Notes[i].UpdatedAt = time.Now()

			return a.saveStructure(structure)
		}
	}

	return fmt.Errorf("note not found")
}

func (a *App) DeleteNote(id string) error {
	structure, err := a.loadStructure()
	if err != nil {
		return err
	}

	for i, note := range structure.Notes {
		if note.ID == id {
			structure.Notes = append(structure.Notes[:i], structure.Notes[i+1:]...)

			if err := a.deleteNoteFile(id, note.FolderID); err != nil {
				return err
			}

			return a.saveStructure(structure)
		}
	}

	return fmt.Errorf("note not found")
}

// ============================================================================
// HELPER FUNCTIONS (unchanged)
// ============================================================================

func (a *App) loadStructure() (*FolderStructure, error) {
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		return nil, err
	}

	var structure FolderStructure
	if err := json.Unmarshal(data, &structure); err != nil {
		return nil, err
	}

	return &structure, nil
}

func (a *App) saveStructure(structure *FolderStructure) error {
	data, err := json.MarshalIndent(structure, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(a.configPath, data, 0644)
}

func (a *App) getFolderPath(folderID string) string {
	if folderID == "" {
		return a.notesPath
	}
	return filepath.Join(a.notesPath, "folders", folderID)
}

func (a *App) ensureFolderExists(folderID string) error {
	folderPath := a.getFolderPath(folderID)
	return os.MkdirAll(folderPath, 0755)
}

func (a *App) loadNoteContent(id string, folderID string) (string, error) {
	var filePath string
	if folderID == "" {
		filePath = filepath.Join(a.notesPath, fmt.Sprintf("%s.md", id))
	} else {
		filePath = filepath.Join(a.getFolderPath(folderID), fmt.Sprintf("%s.md", id))
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) saveNoteContent(id string, folderID string, content string) error {
	var filePath string
	if folderID == "" {
		filePath = filepath.Join(a.notesPath, fmt.Sprintf("%s.md", id))
	} else {
		filePath = filepath.Join(a.getFolderPath(folderID), fmt.Sprintf("%s.md", id))
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (a *App) moveNoteFile(noteID string, oldFolderID string, newFolderID string) error {
	var oldPath, newPath string

	if oldFolderID == "" {
		oldPath = filepath.Join(a.notesPath, fmt.Sprintf("%s.md", noteID))
	} else {
		oldPath = filepath.Join(a.getFolderPath(oldFolderID), fmt.Sprintf("%s.md", noteID))
	}

	if newFolderID == "" {
		newPath = filepath.Join(a.notesPath, fmt.Sprintf("%s.md", noteID))
	} else {
		if err := a.ensureFolderExists(newFolderID); err != nil {
			return err
		}
		newPath = filepath.Join(a.getFolderPath(newFolderID), fmt.Sprintf("%s.md", noteID))
	}

	return os.Rename(oldPath, newPath)
}

func (a *App) deleteNoteFile(id string, folderID string) error {
	var filePath string
	if folderID == "" {
		filePath = filepath.Join(a.notesPath, fmt.Sprintf("%s.md", id))
	} else {
		filePath = filepath.Join(a.getFolderPath(folderID), fmt.Sprintf("%s.md", id))
	}
	return os.Remove(filePath)
}

func (a *App) validateFolderMove(folderID string, newParentID string, structure *FolderStructure) error {
	if folderID == newParentID {
		return fmt.Errorf("cannot move folder into itself")
	}

	currentID := newParentID
	for currentID != "" {
		if currentID == folderID {
			return fmt.Errorf("cannot create circular folder reference")
		}

		found := false
		for _, folder := range structure.Folders {
			if folder.ID == currentID {
				currentID = folder.ParentID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	if newParentID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == newParentID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("parent folder not found")
		}
	}

	return nil
}

// loadConfig loads the application configuration from config.json
func (a *App) loadConfig() error {
	configFilePath := filepath.Join(a.notesPath, "config.json")

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		// Config file does not exist, so create it from the embedded template
		fmt.Printf("config.json not found. Creating from embedded template at %s\n", configFilePath)
		if err := os.WriteFile(configFilePath, defaultConfig, 0644); err != nil {
			return fmt.Errorf("failed to write default config file: %w", err)
		}
		// Use the embedded config data for this session
		data = defaultConfig
	}

	// Unmarshal the config data (either from file or the embedded default)
	var config Config
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
	modelsPath := filepath.Join(a.notesPath, "models")
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
	sttService, err := NewSTTService(a.notesPath, a.config.RealtimeTranscriptionChunkSeconds, a.config.ModelName)
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
