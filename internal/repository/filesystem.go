package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// LoadNoteContentPair reads markdown and transcript note contents.
// Missing files are tolerated and returned as empty strings.
func (fs *FileSystem) LoadNoteContentPair(note *domain.Note, structure *domain.FolderStructure) (string, string, error) {
	markdownPath, err := fs.pathResolver.NoteMarkdownPath(note, structure)
	if err != nil {
		return "", "", fmt.Errorf("could not resolve markdown path for note %q: %w", note.ID, err)
	}
	transcriptPath, err := fs.pathResolver.NoteTranscriptPath(note, structure)
	if err != nil {
		return "", "", fmt.Errorf("could not resolve transcript path for note %q: %w", note.ID, err)
	}

	markdown, err := readFileIfExists(markdownPath)
	if err != nil {
		return "", "", fmt.Errorf("could not read markdown file %q: %w", markdownPath, err)
	}
	transcript, err := readFileIfExists(transcriptPath)
	if err != nil {
		return "", "", fmt.Errorf("could not read transcript file %q: %w", transcriptPath, err)
	}

	return markdown, transcript, nil
}

// SaveNoteContentPair writes markdown and transcript contents as one logical operation.
func (fs *FileSystem) SaveNoteContentPair(note *domain.Note, markdownContent, transcriptContent string, structure *domain.FolderStructure) error {
	markdownPath, err := fs.pathResolver.NoteMarkdownPath(note, structure)
	if err != nil {
		return fmt.Errorf("could not resolve markdown path for note %q: %w", note.ID, err)
	}
	transcriptPath, err := fs.pathResolver.NoteTranscriptPath(note, structure)
	if err != nil {
		return fmt.Errorf("could not resolve transcript path for note %q: %w", note.ID, err)
	}

	if err := os.MkdirAll(filepath.Dir(markdownPath), 0755); err != nil {
		return fmt.Errorf("could not create markdown parent directory for note %q: %w", note.ID, err)
	}
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0755); err != nil {
		return fmt.Errorf("could not create transcript parent directory for note %q: %w", note.ID, err)
	}

	if err := os.WriteFile(markdownPath, []byte(markdownContent), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0644); err != nil {
		return err
	}

	return nil
}

// MoveNoteFilePair moves markdown and transcript note files together.
func (fs *FileSystem) MoveNoteFilePair(
	note *domain.Note,
	oldFolderID, newFolderID string,
	structure *domain.FolderStructure,
) error {
	oldMarkdownParent, err := fs.pathResolver.FolderPathInMarkdownRoot(oldFolderID, structure)
	if err != nil {
		return fmt.Errorf("could not resolve old markdown parent path: %w", err)
	}
	newMarkdownParent, err := fs.pathResolver.FolderPathInMarkdownRoot(newFolderID, structure)
	if err != nil {
		return fmt.Errorf("could not resolve new markdown parent path: %w", err)
	}
	oldTranscriptParent, err := fs.pathResolver.FolderPathInTranscriptRoot(oldFolderID, structure)
	if err != nil {
		return fmt.Errorf("could not resolve old transcript parent path: %w", err)
	}
	newTranscriptParent, err := fs.pathResolver.FolderPathInTranscriptRoot(newFolderID, structure)
	if err != nil {
		return fmt.Errorf("could not resolve new transcript parent path: %w", err)
	}

	oldMarkdownPath := filepath.Join(oldMarkdownParent, fmt.Sprintf("%s.md", note.FileStem))
	newMarkdownPath := filepath.Join(newMarkdownParent, fmt.Sprintf("%s.md", note.FileStem))
	oldTranscriptPath := filepath.Join(oldTranscriptParent, fmt.Sprintf("%s.transcript.txt", note.FileStem))
	newTranscriptPath := filepath.Join(newTranscriptParent, fmt.Sprintf("%s.transcript.txt", note.FileStem))

	if oldMarkdownPath == newMarkdownPath && oldTranscriptPath == newTranscriptPath {
		return nil
	}

	if err := os.MkdirAll(newMarkdownParent, 0755); err != nil {
		return fmt.Errorf("could not create markdown destination directory: %w", err)
	}
	if err := os.MkdirAll(newTranscriptParent, 0755); err != nil {
		return fmt.Errorf("could not create transcript destination directory: %w", err)
	}

	markdownMoved := false
	if err := os.Rename(oldMarkdownPath, newMarkdownPath); err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		markdownMoved = true
	}

	if err := os.Rename(oldTranscriptPath, newTranscriptPath); err != nil && !os.IsNotExist(err) {
		if markdownMoved {
			_ = os.Rename(newMarkdownPath, oldMarkdownPath)
		}
		return err
	}

	return nil
}

// RenameNoteFilePair renames the markdown/transcript filenames for a note while
// keeping both files in the same folder.
func (fs *FileSystem) RenameNoteFilePair(note *domain.Note, newFileStem string, structure *domain.FolderStructure) error {
	if strings.TrimSpace(note.FileStem) == "" {
		return fmt.Errorf("note %q missing file stem", note.ID)
	}
	if strings.TrimSpace(newFileStem) == "" {
		return fmt.Errorf("new file stem cannot be empty")
	}
	if newFileStem == note.FileStem {
		return nil
	}

	oldMarkdownPath, err := fs.pathResolver.NoteMarkdownPath(note, structure)
	if err != nil {
		return fmt.Errorf("could not resolve old markdown path for note %q: %w", note.ID, err)
	}
	oldTranscriptPath, err := fs.pathResolver.NoteTranscriptPath(note, structure)
	if err != nil {
		return fmt.Errorf("could not resolve old transcript path for note %q: %w", note.ID, err)
	}

	renamed := *note
	renamed.FileStem = newFileStem
	newMarkdownPath, err := fs.pathResolver.NoteMarkdownPath(&renamed, structure)
	if err != nil {
		return fmt.Errorf("could not resolve new markdown path for note %q: %w", note.ID, err)
	}
	newTranscriptPath, err := fs.pathResolver.NoteTranscriptPath(&renamed, structure)
	if err != nil {
		return fmt.Errorf("could not resolve new transcript path for note %q: %w", note.ID, err)
	}

	if oldMarkdownPath != newMarkdownPath && fileExists(newMarkdownPath) {
		return fmt.Errorf("target markdown filename already exists: %q", newMarkdownPath)
	}
	if oldTranscriptPath != newTranscriptPath && fileExists(newTranscriptPath) {
		return fmt.Errorf("target transcript filename already exists: %q", newTranscriptPath)
	}

	markdownRenamed := false
	if err := os.Rename(oldMarkdownPath, newMarkdownPath); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		markdownRenamed = true
	}

	if err := os.Rename(oldTranscriptPath, newTranscriptPath); err != nil {
		if !os.IsNotExist(err) {
			if markdownRenamed {
				_ = os.Rename(newMarkdownPath, oldMarkdownPath)
			}
			return err
		}
	}

	return nil
}

// DeleteNoteFilePair removes markdown and transcript files.
func (fs *FileSystem) DeleteNoteFilePair(note *domain.Note, structure *domain.FolderStructure) error {
	markdownPath, err := fs.pathResolver.NoteMarkdownPath(note, structure)
	if err != nil {
		return fmt.Errorf("could not resolve markdown path for note %q: %w", note.ID, err)
	}
	transcriptPath, err := fs.pathResolver.NoteTranscriptPath(note, structure)
	if err != nil {
		return fmt.Errorf("could not resolve transcript path for note %q: %w", note.ID, err)
	}

	if err := os.Remove(markdownPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(transcriptPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func readFileIfExists(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}
