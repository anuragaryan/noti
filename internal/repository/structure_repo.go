package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"noti/internal/domain"
)

// StructureRepository handles persistence of folder/note structure
type StructureRepository struct {
	structurePath string
}

type persistedFolderStructure struct {
	SchemaVersion int             `json:"schemaVersion"`
	Folders       []domain.Folder `json:"folders"`
	Notes         []persistedNote `json:"notes"`
}

type persistedNote struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	FileStem            string `json:"fileStem"`
	FolderID            string `json:"folderId"`
	TranscriptActivated bool   `json:"transcriptActivated"`
	CreatedAt           any    `json:"createdAt"`
	UpdatedAt           any    `json:"updatedAt"`
	Order               int    `json:"order"`
}

// NewStructureRepository creates a new structure repository
func NewStructureRepository(structurePath string) *StructureRepository {
	return &StructureRepository{
		structurePath: structurePath,
	}
}

// Load reads the folder structure from disk
func (r *StructureRepository) Load() (*domain.FolderStructure, error) {
	data, err := os.ReadFile(r.structurePath)
	if err != nil {
		return nil, err
	}

	var structure domain.FolderStructure
	if err := json.Unmarshal(data, &structure); err != nil {
		return nil, err
	}

	return &structure, nil
}

// Save writes the folder structure to disk
func (r *StructureRepository) Save(structure *domain.FolderStructure) error {
	// Ensure the directory for structure.json exists
	dir := filepath.Dir(r.structurePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for structure.json: %w", err)
	}

	persisted := metadataOnlyStructure(structure)

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.structurePath, data, 0644)
}

func metadataOnlyStructure(structure *domain.FolderStructure) *persistedFolderStructure {
	if structure == nil {
		return &persistedFolderStructure{Folders: []domain.Folder{}, Notes: []persistedNote{}}
	}

	persisted := &persistedFolderStructure{
		SchemaVersion: structure.SchemaVersion,
		Folders:       append([]domain.Folder{}, structure.Folders...),
		Notes:         make([]persistedNote, len(structure.Notes)),
	}

	for i := range structure.Notes {
		note := structure.Notes[i]
		persisted.Notes[i] = persistedNote{
			ID:                  note.ID,
			Title:               note.Title,
			FileStem:            note.FileStem,
			FolderID:            note.FolderID,
			TranscriptActivated: note.TranscriptActivated,
			CreatedAt:           note.CreatedAt,
			UpdatedAt:           note.UpdatedAt,
			Order:               note.Order,
		}
	}

	return persisted
}
