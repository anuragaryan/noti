package repository

import (
	"fmt"
	"os"
	"path/filepath"

	"noti/internal/domain"
)

// FileSystem handles file operations for notes
type FileSystem struct {
	pathResolver *PathResolver
}

// NewFileSystem creates a new filesystem handler
func NewFileSystem(pathResolver *PathResolver) *FileSystem {
	return &FileSystem{
		pathResolver: pathResolver,
	}
}

// LoadNoteContent reads note content from disk
func (fs *FileSystem) LoadNoteContent(note *domain.Note, structure *domain.FolderStructure) (string, error) {
	filePath, err := fs.pathResolver.GetPathFor(note.ID, structure)
	if err != nil {
		return "", fmt.Errorf("could not get path for note %s: %w", note.ID, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// SaveNoteContent writes note content to disk
func (fs *FileSystem) SaveNoteContent(note *domain.Note, content string, structure *domain.FolderStructure) error {
	filePath, err := fs.pathResolver.GetPathFor(note.ID, structure)
	if err != nil {
		return fmt.Errorf("could not get path for note %s: %w", note.ID, err)
	}

	dirPath := filepath.Dir(filePath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("could not create directory for note: %w", err)
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

// MoveNoteFile moves a note file from one folder to another
func (fs *FileSystem) MoveNoteFile(note *domain.Note, oldFolderID string, newFolderID string, structure *domain.FolderStructure, notesPath string) error {
	oldParentPath := notesPath
	if oldFolderID != "" {
		var err error
		oldParentPath, err = fs.pathResolver.GetPathFor(oldFolderID, structure)
		if err != nil {
			return fmt.Errorf("could not resolve old parent path: %w", err)
		}
	}
	oldPath := filepath.Join(oldParentPath, note.NameOnDisk)

	newParentPath := notesPath
	if newFolderID != "" {
		var err error
		newParentPath, err = fs.pathResolver.GetPathFor(newFolderID, structure)
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

// DeleteNoteFile removes a note file from disk
func (fs *FileSystem) DeleteNoteFile(note *domain.Note, structure *domain.FolderStructure) error {
	filePath, err := fs.pathResolver.GetPathFor(note.ID, structure)
	if err != nil {
		return fmt.Errorf("could not get path for note %s: %w", note.ID, err)
	}
	return os.Remove(filePath)
}
