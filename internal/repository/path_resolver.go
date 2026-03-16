package repository

import (
	"fmt"
	"path/filepath"

	"noti/internal/domain"
)

// PathResolver resolves the absolute filesystem path for any note or folder ID.
type PathResolver struct {
	notesPath      string
	markdownRoot   string
	transcriptRoot string
}

// NewPathResolver creates a PathResolver rooted at notesPath.
func NewPathResolver(notesPath string) *PathResolver {
	return &PathResolver{
		notesPath:      notesPath,
		markdownRoot:   filepath.Join(notesPath, "markdown"),
		transcriptRoot: filepath.Join(notesPath, "transcripts"),
	}
}

func (r *PathResolver) MarkdownRootPath() string {
	return r.markdownRoot
}

func (r *PathResolver) TranscriptRootPath() string {
	return r.transcriptRoot

}

// GetPathFor resolves note or folder absolute paths from the split storage model.
func (r *PathResolver) GetPathFor(id string, structure *domain.FolderStructure) (string, error) {
	if id == "" {
		return r.notesPath, nil
	}
	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			if structure.Notes[i].FileStem == "" {
				return "", fmt.Errorf("note %q missing file stem", id)
			}
			return r.NoteMarkdownPath(&structure.Notes[i], structure)
		}
	}
	rel, err := r.FolderRelativePath(id, structure)
	if err != nil {
		return "", err
	}
	return filepath.Join(r.notesPath, rel), nil
}

func (r *PathResolver) FolderPathInMarkdownRoot(folderID string, structure *domain.FolderStructure) (string, error) {
	rel, err := r.folderRelativePath(folderID, structure, make(map[string]bool))
	if err != nil {
		return "", err
	}
	if rel == "" {
		return r.markdownRoot, nil
	}
	return filepath.Join(r.markdownRoot, rel), nil
}

func (r *PathResolver) FolderPathInTranscriptRoot(folderID string, structure *domain.FolderStructure) (string, error) {
	rel, err := r.folderRelativePath(folderID, structure, make(map[string]bool))
	if err != nil {
		return "", err
	}
	if rel == "" {
		return r.transcriptRoot, nil
	}
	return filepath.Join(r.transcriptRoot, rel), nil
}

func (r *PathResolver) NoteMarkdownPath(note *domain.Note, structure *domain.FolderStructure) (string, error) {
	parent, err := r.FolderPathInMarkdownRoot(note.FolderID, structure)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, fmt.Sprintf("%s.md", note.FileStem)), nil
}

func (r *PathResolver) NoteTranscriptPath(note *domain.Note, structure *domain.FolderStructure) (string, error) {
	parent, err := r.FolderPathInTranscriptRoot(note.FolderID, structure)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, fmt.Sprintf("%s.transcript.txt", note.FileStem)), nil
}

func (r *PathResolver) FolderRelativePath(folderID string, structure *domain.FolderStructure) (string, error) {
	return r.folderRelativePath(folderID, structure, make(map[string]bool))
}

func (r *PathResolver) folderRelativePath(folderID string, structure *domain.FolderStructure, visited map[string]bool) (string, error) {
	if folderID == "" {
		return "", nil
	}
	if visited[folderID] {
		return "", fmt.Errorf("circular reference detected for folder %q", folderID)
	}
	visited[folderID] = true

	for _, folder := range structure.Folders {
		if folder.ID != folderID {
			continue
		}
		if folder.ParentID == "" {
			return folder.NameOnDisk, nil
		}
		parentRel, err := r.folderRelativePath(folder.ParentID, structure, visited)
		if err != nil {
			return "", err
		}
		return filepath.Join(parentRel, folder.NameOnDisk), nil
	}

	return "", fmt.Errorf("folder ID %q not found", folderID)
}
