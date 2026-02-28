package repository

import (
	"fmt"
	"path/filepath"

	"noti/internal/domain"
)

// PathResolver resolves the absolute filesystem path for any note or folder ID.
type PathResolver struct {
	notesPath string
}

// NewPathResolver creates a PathResolver rooted at notesPath.
func NewPathResolver(notesPath string) *PathResolver {
	return &PathResolver{notesPath: notesPath}
}

// GetPathFor returns the absolute disk path for the given folder or note ID.
//
// It walks up the parent chain recursively. A visited set is kept at every
// call site so that a corrupted structure with circular parent references
// causes a clear error instead of an infinite recursion / stack overflow.
func (r *PathResolver) GetPathFor(id string, structure *domain.FolderStructure) (string, error) {
	return r.resolvePath(id, structure, make(map[string]bool))
}

// resolvePath is the internal recursive implementation. visited tracks every
// ID seen on the current call stack to detect cycles.
func (r *PathResolver) resolvePath(id string, structure *domain.FolderStructure, visited map[string]bool) (string, error) {
	if visited[id] {
		return "", fmt.Errorf("circular reference detected for ID %q: the parent chain forms a cycle", id)
	}
	visited[id] = true

	// Check notes first.
	for _, note := range structure.Notes {
		if note.ID != id {
			continue
		}
		if note.FolderID == "" {
			return filepath.Join(r.notesPath, note.NameOnDisk), nil
		}
		parentPath, err := r.resolvePath(note.FolderID, structure, visited)
		if err != nil {
			return "", fmt.Errorf("could not resolve parent folder %q for note %q: %w", note.FolderID, id, err)
		}
		return filepath.Join(parentPath, note.NameOnDisk), nil
	}

	// Check folders.
	for _, folder := range structure.Folders {
		if folder.ID != id {
			continue
		}
		if folder.ParentID == "" {
			return filepath.Join(r.notesPath, folder.NameOnDisk), nil
		}
		parentPath, err := r.resolvePath(folder.ParentID, structure, visited)
		if err != nil {
			return "", fmt.Errorf("could not resolve parent folder %q for folder %q: %w", folder.ParentID, id, err)
		}
		return filepath.Join(parentPath, folder.NameOnDisk), nil
	}

	return "", fmt.Errorf("ID %q not found in structure", id)
}
