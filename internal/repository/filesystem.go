package repository

import (
	"fmt"
	"os"
	"path/filepath"

	"noti/internal/domain"
)

// FileSystem handles all file I/O for note content.
type FileSystem struct {
	pathResolver *PathResolver
}

// NewFileSystem creates a FileSystem backed by the given PathResolver.
func NewFileSystem(pathResolver *PathResolver) *FileSystem {
	return &FileSystem{pathResolver: pathResolver}
}

// LoadNoteContent reads and returns the raw text content of a note from disk.
func (fs *FileSystem) LoadNoteContent(note *domain.Note, structure *domain.FolderStructure) (string, error) {
	filePath, err := fs.pathResolver.GetPathFor(note.ID, structure)
	if err != nil {
		return "", fmt.Errorf("could not resolve path for note %q: %w", note.ID, err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("could not read note file %q: %w", filePath, err)
	}
	return string(data), nil
}

// SaveNoteContent writes content to the note's file, creating parent
// directories if they do not yet exist.
func (fs *FileSystem) SaveNoteContent(note *domain.Note, content string, structure *domain.FolderStructure) error {
	filePath, err := fs.pathResolver.GetPathFor(note.ID, structure)
	if err != nil {
		return fmt.Errorf("could not resolve path for note %q: %w", note.ID, err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("could not create parent directory for note %q: %w", note.ID, err)
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

// MoveNoteFile moves a note's file from oldFolderID's directory to
// newFolderID's directory (either may be empty to indicate the notes root).
func (fs *FileSystem) MoveNoteFile(
	note *domain.Note,
	oldFolderID, newFolderID string,
	structure *domain.FolderStructure,
	notesPath string,
) error {
	oldParentPath, err := fs.resolveParentPath(oldFolderID, structure, notesPath)
	if err != nil {
		return fmt.Errorf("could not resolve old parent path: %w", err)
	}

	newParentPath, err := fs.resolveParentPath(newFolderID, structure, notesPath)
	if err != nil {
		return fmt.Errorf("could not resolve new parent path: %w", err)
	}

	oldPath := filepath.Join(oldParentPath, note.NameOnDisk)
	newPath := filepath.Join(newParentPath, note.NameOnDisk)

	if oldPath == newPath {
		return nil // nothing to do
	}

	if err := os.MkdirAll(newParentPath, 0755); err != nil {
		return fmt.Errorf("could not create destination directory: %w", err)
	}

	return os.Rename(oldPath, newPath)
}

// DeleteNoteFile removes a note's file from disk.
func (fs *FileSystem) DeleteNoteFile(note *domain.Note, structure *domain.FolderStructure) error {
	filePath, err := fs.pathResolver.GetPathFor(note.ID, structure)
	if err != nil {
		return fmt.Errorf("could not resolve path for note %q: %w", note.ID, err)
	}
	return os.Remove(filePath)
}

// resolveParentPath returns the absolute path for a folder ID. An empty ID
// means the notes root directory.
func (fs *FileSystem) resolveParentPath(folderID string, structure *domain.FolderStructure, notesPath string) (string, error) {
	if folderID == "" {
		return notesPath, nil
	}
	return fs.pathResolver.GetPathFor(folderID, structure)
}
