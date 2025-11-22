package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// App struct
type App struct {
	ctx        context.Context
	configPath string
	notesPath  string
	sttService *STTService
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

// NewApp creates a new App application struct
func NewApp() *App {
	homeDir, _ := os.UserHomeDir()
	notesPath := filepath.Join(homeDir, "MarkdownNotes")

	return &App{
		configPath: filepath.Join(notesPath, "structure.json"),
		notesPath:  notesPath,
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Create notes directory if it doesn't exist
	if err := os.MkdirAll(a.notesPath, 0755); err != nil {
		fmt.Printf("Error creating notes directory: %v\n", err)
	}

	// Create models directory
	modelsPath := filepath.Join(a.notesPath, "models")
	if err := os.MkdirAll(modelsPath, 0755); err != nil {
		fmt.Printf("Error creating models directory: %v\n", err)
	}

	// Create structure.json if it doesn't exist
	if _, err := os.Stat(a.configPath); os.IsNotExist(err) {
		a.saveStructure(&FolderStructure{
			Folders: []Folder{},
			Notes:   []Note{},
		})
	}

	// Initialize STT Service
	sttService, err := NewSTTService(a.notesPath)
	if err != nil {
		fmt.Printf("Warning: STT service initialization failed: %v\n", err)
		fmt.Println("Speech-to-text features will be disabled")
		fmt.Printf("To enable STT, download the Whisper model to: %s/models/ggml-base.en.bin\n", a.notesPath)
	} else {
		if err := sttService.Initialize(); err != nil {
			fmt.Printf("Warning: Failed to load STT model: %v\n", err)
			fmt.Println("Speech-to-text features will be disabled")
		} else {
			sttService.SetContext(ctx) // Set context for real-time events
			a.sttService = sttService
			fmt.Println("STT service initialized successfully with real-time transcription")
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
	modelPath := filepath.Join(a.notesPath, "models", "ggml-base.en.bin")

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
		return nil, err
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
		return nil, err
	}

	structure.Folders = append(structure.Folders, folder)
	if err := a.saveStructure(structure); err != nil {
		return nil, err
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
		return nil, err
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
			return nil, err
		}
	}

	if err := a.saveNoteContent(note.ID, folderID, content); err != nil {
		return nil, err
	}

	structure.Notes = append(structure.Notes, note)
	if err := a.saveStructure(structure); err != nil {
		return nil, err
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
