package repository

import (
	"fmt"
	"path/filepath"

	"noti/internal/domain"
)

// PathResolver handles path resolution for folders and notes
type PathResolver struct {
	notesPath string
}

// NewPathResolver creates a new path resolver
func NewPathResolver(notesPath string) *PathResolver {
	return &PathResolver{
		notesPath: notesPath,
	}
}

// GetPathFor resolves the full disk path for a given folder or note ID
func (r *PathResolver) GetPathFor(id string, structure *domain.FolderStructure) (string, error) {
	// Check if it's a note
	for _, note := range structure.Notes {
		if note.ID == id {
			if note.FolderID == "" {
				return filepath.Join(r.notesPath, note.NameOnDisk), nil
			}
			parentPath, err := r.GetPathFor(note.FolderID, structure)
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
				return filepath.Join(r.notesPath, folder.NameOnDisk), nil
			}
			parentPath, err := r.GetPathFor(folder.ParentID, structure)
			if err != nil {
				return "", err
			}
			return filepath.Join(parentPath, folder.NameOnDisk), nil
		}
	}

	return "", fmt.Errorf("ID %s not found in structure", id)
}
