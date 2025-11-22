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
	configPath string
}

// NewStructureRepository creates a new structure repository
func NewStructureRepository(configPath string) *StructureRepository {
	return &StructureRepository{
		configPath: configPath,
	}
}

// Load reads the folder structure from disk
func (r *StructureRepository) Load() (*domain.FolderStructure, error) {
	data, err := os.ReadFile(r.configPath)
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
	dir := filepath.Dir(r.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for structure.json: %w", err)
	}

	data, err := json.MarshalIndent(structure, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.configPath, data, 0644)
}
