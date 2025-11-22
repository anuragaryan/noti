package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	basePath   string
	configPath string
	notesPath  string
	sttService *STTService
	config     *Config
}

// Folder represents a folder/category
type Folder struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	NameOnDisk string    `json:"nameOnDisk"`
	ParentID   string    `json:"parentId"`
	CreatedAt  time.Time `json:"createdAt"`
	Order      int       `json:"order"`
}

// Note represents a note entry
type Note struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	NameOnDisk string    `json:"nameOnDisk"`
	FolderID   string    `json:"folderId"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Order      int       `json:"order"`
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
	basePath := filepath.Join(homeDir, "Documents", "Noti")
	notesPath := filepath.Join(basePath, "notes")

	return &App{
		basePath:   basePath,
		configPath: filepath.Join(notesPath, "structure.json"),
		notesPath:  notesPath,
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
	modelPath := filepath.Join(a.basePath, "models", modelFileName)

	return map[string]interface{}{
		"available": available,
		"modelPath": modelPath,
	}
}

// ============================================================================
// FOLDER OPERATIONS
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

	for i, folder := range structure.Folders {
		if folder.ID == id {
			return &structure.Folders[i], nil
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
		ID:         fmt.Sprintf("f_%d", now.UnixNano()),
		Name:       name,
		NameOnDisk: generateNameOnDisk(name),
		ParentID:   parentID,
		CreatedAt:  now,
		Order:      len(structure.Folders),
	}

	parentPath := a.notesPath
	if parentID != "" {
		var err error
		parentPath, err = a.getPathFor(parentID, structure)
		if err != nil {
			return nil, fmt.Errorf("could not resolve parent path: %v", err)
		}
	}

	newFolderPath := filepath.Join(parentPath, folder.NameOnDisk)
	if err := os.MkdirAll(newFolderPath, 0755); err != nil {
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

	for i := range structure.Folders {
		if structure.Folders[i].ID == id {
			// If the name is different, we need to rename the folder on disk
			if structure.Folders[i].Name != name {
				oldPath, err := a.getPathFor(id, structure)
				if err != nil {
					return fmt.Errorf("could not resolve old path for folder rename: %w", err)
				}

				// Keep the original timestamp, just update the name part
				parts := strings.SplitN(structure.Folders[i].NameOnDisk, "-", 2)
				timestamp := parts[0]
				newDiskName := fmt.Sprintf("%s-%s", timestamp, SanitizeName(name))

				parentPath := a.notesPath
				if structure.Folders[i].ParentID != "" {
					parentPath, err = a.getPathFor(structure.Folders[i].ParentID, structure)
					if err != nil {
						return fmt.Errorf("could not resolve parent path for rename: %w", err)
					}
				}
				newPath := filepath.Join(parentPath, newDiskName)

				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("failed to rename folder on disk: %w", err)
				}
				structure.Folders[i].NameOnDisk = newDiskName
			}

			structure.Folders[i].Name = name
			if parentID != structure.Folders[i].ParentID {
				// Moving folders is not implemented in this refactor, but we keep the check
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

	// Find the folder to get its path BEFORE removing it from the structure
	folderPath, err := a.getPathFor(id, structure)
	if err != nil {
		// If we can't get the path, we can't delete it. Log it and continue.
		fmt.Printf("Warning: could not resolve path for folder %s to delete it from disk: %v\n", id, err)
	}

	for _, folder := range structure.Folders {
		if folder.ParentID == id {
			return fmt.Errorf("cannot delete folder with subfolders")
		}
	}

	if deleteNotes {
		var notesInFolder []*Note
		var remainingNotes []Note
		for i := range structure.Notes {
			if structure.Notes[i].FolderID == id {
				notesInFolder = append(notesInFolder, &structure.Notes[i])
			} else {
				remainingNotes = append(remainingNotes, structure.Notes[i])
			}
		}

		for _, note := range notesInFolder {
			if err := a.deleteNoteFile(note, structure); err != nil {
				// Log error but continue trying to delete other notes
				fmt.Printf("Warning: failed to delete note file %s: %v\n", note.ID, err)
			}
		}
		structure.Notes = remainingNotes

	} else {
		// Move notes to root
		for i := range structure.Notes {
			if structure.Notes[i].FolderID == id {
				if err := a.moveNoteFile(&structure.Notes[i], id, "", structure); err != nil {
					return fmt.Errorf("failed to move note out of deleted folder: %w", err)
				}
				structure.Notes[i].FolderID = ""
			}
		}
	}

	// Now remove the folder from the structure
	newFolders := []Folder{}
	for _, folder := range structure.Folders {
		if folder.ID != id {
			newFolders = append(newFolders, folder)
		}
	}
	structure.Folders = newFolders

	// And finally, remove the folder from disk if the path was found
	if err == nil { // Only try to remove if getPathFor succeeded
		if err := os.RemoveAll(folderPath); err != nil {
			return fmt.Errorf("failed to delete folder directory '%s': %w", folderPath, err)
		}
	}

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
// NOTE OPERATIONS
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

	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			content, err := a.loadNoteContent(&structure.Notes[i], structure)
			if err != nil {
				// If content fails to load, maybe the file is missing.
				// Return the note without content, but log the error.
				fmt.Printf("Warning: could not load content for note %s: %v\n", id, err)
				structure.Notes[i].Content = "" // Ensure content is empty
			} else {
				structure.Notes[i].Content = content
			}
			return &structure.Notes[i], nil
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
		ID:         fmt.Sprintf("%d", now.UnixNano()),
		Title:      title,
		NameOnDisk: generateNameOnDisk(title) + ".md",
		FolderID:   folderID,
		Content:    content,
		CreatedAt:  now,
		UpdatedAt:  now,
		Order:      len(structure.Notes),
	}

	// Manually construct path for the new note and save it.
	parentPath := a.notesPath
	if folderID != "" {
		var err error
		parentPath, err = a.getPathFor(folderID, structure)
		if err != nil {
			return nil, fmt.Errorf("could not resolve parent path for note: %v", err)
		}
	}
	notePath := filepath.Join(parentPath, note.NameOnDisk)
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to save new note content: %v", err)
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

	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			// If title is different, rename the file on disk
			if structure.Notes[i].Title != title {
				oldPath, err := a.getPathFor(id, structure)
				if err != nil {
					return fmt.Errorf("could not resolve old path for note update: %w", err)
				}

				parts := strings.SplitN(structure.Notes[i].NameOnDisk, "-", 2)
				timestamp := parts[0]
				newDiskName := fmt.Sprintf("%s-%s.md", timestamp, SanitizeName(title))

				parentPath := a.notesPath
				if structure.Notes[i].FolderID != "" {
					parentPath, err = a.getPathFor(structure.Notes[i].FolderID, structure)
					if err != nil {
						return fmt.Errorf("could not resolve parent path for note update: %w", err)
					}
				}
				newPath := filepath.Join(parentPath, newDiskName)

				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("failed to rename note on disk: %w", err)
				}
				structure.Notes[i].NameOnDisk = newDiskName
			}

			structure.Notes[i].Title = title
			structure.Notes[i].UpdatedAt = time.Now()

			// Save the content to the (potentially new) path
			if err := a.saveNoteContent(&structure.Notes[i], content, structure); err != nil {
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

	for i := range structure.Notes {
		if structure.Notes[i].ID == noteID {
			oldFolderID := structure.Notes[i].FolderID

			if err := a.moveNoteFile(&structure.Notes[i], oldFolderID, targetFolderID, structure); err != nil {
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

	var noteToDelete *Note
	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			noteToDelete = &structure.Notes[i]
			break
		}
	}

	if noteToDelete == nil {
		return fmt.Errorf("note not found")
	}

	// If deleting the file fails, abort the entire operation.
	if err := a.deleteNoteFile(noteToDelete, structure); err != nil {
		return fmt.Errorf("failed to delete note file from disk: %w", err)
	}

	newNotes := []Note{}
	for _, note := range structure.Notes {
		if note.ID != id {
			newNotes = append(newNotes, note)
		}
	}
	structure.Notes = newNotes

	return a.saveStructure(structure)
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// SanitizeName removes characters that are problematic for file systems.
func SanitizeName(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ToLower(name)
	// Basic sanitization
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		return "untitled"
	}
	return name
}

// generateNameOnDisk creates a filesystem-friendly name with a timestamp.
func generateNameOnDisk(name string) string {
	now := time.Now().Unix()
	return fmt.Sprintf("%d-%s", now, SanitizeName(name))
}

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
	// Ensure the directory for structure.json exists.
	dir := filepath.Dir(a.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for structure.json: %w", err)
	}

	data, err := json.MarshalIndent(structure, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(a.configPath, data, 0644)
}

// getPathFor resolves the full disk path for a given folder or note ID.
func (a *App) getPathFor(id string, structure *FolderStructure) (string, error) {
	// Check if it's a note
	for _, note := range structure.Notes {
		if note.ID == id {
			if note.FolderID == "" {
				return filepath.Join(a.notesPath, note.NameOnDisk), nil
			}
			parentPath, err := a.getPathFor(note.FolderID, structure)
			if err != nil {
				return "", err
			}
			return filepath.Join(parentPath, note.NameOnDisk), nil
		}
	}

	// Check if it's a folder
	for _, folder := range structure.Folders {
		if folder.ID == id {
			if folder.ParentID == "" {
				return filepath.Join(a.notesPath, folder.NameOnDisk), nil
			}
			parentPath, err := a.getPathFor(folder.ParentID, structure)
			if err != nil {
				return "", err
			}
			return filepath.Join(parentPath, folder.NameOnDisk), nil
		}
	}

	return "", fmt.Errorf("ID %s not found in structure", id)
}

func (a *App) loadNoteContent(note *Note, structure *FolderStructure) (string, error) {
	filePath, err := a.getPathFor(note.ID, structure)
	if err != nil {
		return "", fmt.Errorf("could not get path for note %s: %w", note.ID, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) saveNoteContent(note *Note, content string, structure *FolderStructure) error {
	filePath, err := a.getPathFor(note.ID, structure)
	if err != nil {
		return fmt.Errorf("could not get path for note %s: %w", note.ID, err)
	}

	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("could not create directory for note: %w", err)
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (a *App) moveNoteFile(note *Note, oldFolderID string, newFolderID string, structure *FolderStructure) error {
	oldParentPath := a.notesPath
	if oldFolderID != "" {
		var err error
		oldParentPath, err = a.getPathFor(oldFolderID, structure)
		if err != nil {
			return fmt.Errorf("could not resolve old parent path: %w", err)
		}
	}
	oldPath := filepath.Join(oldParentPath, note.NameOnDisk)

	newParentPath := a.notesPath
	if newFolderID != "" {
		var err error
		newParentPath, err = a.getPathFor(newFolderID, structure)
		if err != nil {
			return fmt.Errorf("could not resolve new parent path: %w", err)
		}
	}
	newPath := filepath.Join(newParentPath, note.NameOnDisk)

	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return fmt.Errorf("could not create new parent directory: %w", err)
	}

	return os.Rename(oldPath, newPath)
}

func (a *App) deleteNoteFile(note *Note, structure *FolderStructure) error {
	filePath, err := a.getPathFor(note.ID, structure)
	if err != nil {
		return fmt.Errorf("could not get path for note %s: %w", note.ID, err)
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
