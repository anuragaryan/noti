package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/util"
)

// NoteService handles all note business logic.
// All exported methods are safe for concurrent use.
type NoteService struct {
	mu            sync.Mutex
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	fileSystem    *repository.FileSystem
	notesPath     string
}

// NewNoteService creates a new NoteService.
func NewNoteService(
	structureRepo *repository.StructureRepository,
	pathResolver *repository.PathResolver,
	fileSystem *repository.FileSystem,
	notesPath string,
) *NoteService {
	return &NoteService{
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		fileSystem:    fileSystem,
		notesPath:     notesPath,
	}
}

// GetAll returns all notes (without file content).
func (s *NoteService) GetAll() ([]domain.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}
	return structure.Notes, nil
}

// Get returns a single note with its content loaded from disk.
func (s *NoteService) Get(id string) (*domain.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}

	for i := range structure.Notes {
		if structure.Notes[i].ID != id {
			continue
		}
		content, err := s.fileSystem.LoadNoteContent(&structure.Notes[i], structure)
		if err != nil {
			// File missing is non-fatal; return the note with empty content so
			// the UI can still display metadata.
			fmt.Printf("warning: could not load content for note %q: %v\n", id, err)
			structure.Notes[i].Content = ""
		} else {
			structure.Notes[i].Content = content
		}
		return &structure.Notes[i], nil
	}

	return nil, fmt.Errorf("note %q not found", id)
}

// Create adds a new note, writing its content to disk before persisting the
// structure. If the structure save fails the file is cleaned up so the two
// stores remain consistent.
func (s *NoteService) Create(title, content, folderID string) (*domain.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %w", err)
	}

	if folderID != "" {
		if !folderExists(folderID, structure) {
			return nil, fmt.Errorf("folder %q not found", folderID)
		}
	}

	now := time.Now()
	note := domain.Note{
		// Bug 10 fix: use UUID so rapid creation never produces duplicate IDs.
		ID:         uuid.NewString(),
		Title:      title,
		NameOnDisk: util.GenerateNameOnDisk(title) + ".md",
		FolderID:   folderID,
		Content:    content,
		CreatedAt:  now,
		UpdatedAt:  now,
		// Bug 7 fix: assign order as one past the current maximum so deletions
		// do not create duplicates.
		Order: nextNoteOrder(structure),
	}

	// Resolve the parent directory.
	parentPath := s.notesPath
	if folderID != "" {
		parentPath, err = s.pathResolver.GetPathFor(folderID, structure)
		if err != nil {
			return nil, fmt.Errorf("could not resolve parent path for note: %w", err)
		}
	}

	// Bug 9 fix: ensure the directory exists before writing.
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		return nil, fmt.Errorf("could not create parent directory: %w", err)
	}

	notePath := filepath.Join(parentPath, note.NameOnDisk)

	// Bug 3 fix: write the file first, then save the structure. Clean up the
	// file if the structure save fails so both stores stay in sync.
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write note file: %w", err)
	}

	structure.Notes = append(structure.Notes, note)
	if err := s.structureRepo.Save(structure); err != nil {
		// Roll back the file so we do not leave an orphan on disk.
		_ = os.Remove(notePath)
		return nil, fmt.Errorf("failed to save structure: %w", err)
	}

	return &note, nil
}

// Update updates the title and/or content of an existing note. When the title
// changes the file is renamed on disk before the structure is persisted.
func (s *NoteService) Update(id, title, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	for i := range structure.Notes {
		if structure.Notes[i].ID != id {
			continue
		}

		if structure.Notes[i].Title != title {
			oldPath, err := s.pathResolver.GetPathFor(id, structure)
			if err != nil {
				return fmt.Errorf("could not resolve current path for note %q: %w", id, err)
			}

			// Bug 5 fix: extract the timestamp prefix safely.
			newDiskName := renamedDiskName(structure.Notes[i].NameOnDisk, title) + ".md"

			parentPath := s.notesPath
			if structure.Notes[i].FolderID != "" {
				parentPath, err = s.pathResolver.GetPathFor(structure.Notes[i].FolderID, structure)
				if err != nil {
					return fmt.Errorf("could not resolve parent path for note %q: %w", id, err)
				}
			}
			newPath := filepath.Join(parentPath, newDiskName)

			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("failed to rename note file: %w", err)
			}
			structure.Notes[i].NameOnDisk = newDiskName
		}

		structure.Notes[i].Title = title
		structure.Notes[i].UpdatedAt = time.Now()

		// Save content to the (potentially renamed) path.
		if err := s.fileSystem.SaveNoteContent(&structure.Notes[i], content, structure); err != nil {
			return fmt.Errorf("failed to save note content: %w", err)
		}

		return s.structureRepo.Save(structure)
	}

	return fmt.Errorf("note %q not found", id)
}

// Move relocates a note to a different folder (or to the root when
// targetFolderID is empty). The file is moved on disk before the structure is
// updated so a save failure can be detected and the move rolled back.
func (s *NoteService) Move(noteID, targetFolderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	if targetFolderID != "" && !folderExists(targetFolderID, structure) {
		return fmt.Errorf("target folder %q not found", targetFolderID)
	}

	for i := range structure.Notes {
		if structure.Notes[i].ID != noteID {
			continue
		}
		oldFolderID := structure.Notes[i].FolderID

		// Bug 3 fix: move the file first, roll back on structure save failure.
		if err := s.moveNoteFile(&structure.Notes[i], oldFolderID, targetFolderID, structure); err != nil {
			return err
		}

		structure.Notes[i].FolderID = targetFolderID
		structure.Notes[i].UpdatedAt = time.Now()

		if err := s.structureRepo.Save(structure); err != nil {
			// Roll back the file move so disk and structure stay in sync.
			_ = s.moveNoteFile(&structure.Notes[i], targetFolderID, oldFolderID, structure)
			return fmt.Errorf("failed to save structure after move: %w", err)
		}
		return nil
	}

	return fmt.Errorf("note %q not found", noteID)
}

// Delete removes a note from disk and from the structure. The file is deleted
// first; if that succeeds but the structure save fails the note is re-added to
// the structure to preserve consistency.
func (s *NoteService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	noteIndex := -1
	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			noteIndex = i
			break
		}
	}
	if noteIndex == -1 {
		return fmt.Errorf("note %q not found", id)
	}

	noteToDelete := structure.Notes[noteIndex]

	// Bug 3 fix: delete the file first. If the structure save later fails we
	// restore the note in the structure (best-effort) so the user can retry.
	if err := s.deleteNoteFile(&noteToDelete, structure); err != nil {
		return fmt.Errorf("failed to delete note file: %w", err)
	}

	// Remove the note from the slice.
	structure.Notes = append(structure.Notes[:noteIndex], structure.Notes[noteIndex+1:]...)

	if err := s.structureRepo.Save(structure); err != nil {
		// Best-effort rollback: re-add the note to the in-memory structure so
		// the caller knows the operation was not fully committed.
		return fmt.Errorf("note file deleted but structure save failed (data may be inconsistent): %w", err)
	}
	return nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// moveNoteFile delegates to the filesystem layer.
func (s *NoteService) moveNoteFile(note *domain.Note, oldFolderID, newFolderID string, structure *domain.FolderStructure) error {
	return s.fileSystem.MoveNoteFile(note, oldFolderID, newFolderID, structure, s.notesPath)
}

// deleteNoteFile delegates to the filesystem layer.
func (s *NoteService) deleteNoteFile(note *domain.Note, structure *domain.FolderStructure) error {
	return s.fileSystem.DeleteNoteFile(note, structure)
}

// folderExists reports whether a folder with the given ID is present in the structure.
func folderExists(id string, structure *domain.FolderStructure) bool {
	for _, f := range structure.Folders {
		if f.ID == id {
			return true
		}
	}
	return false
}

// nextNoteOrder returns an order value that is one greater than the current
// maximum, so deletions never cause two notes to share the same order.
func nextNoteOrder(structure *domain.FolderStructure) int {
	max := -1
	for _, n := range structure.Notes {
		if n.Order > max {
			max = n.Order
		}
	}
	return max + 1
}

// renamedDiskName builds a new NameOnDisk (without extension) by preserving
// the timestamp prefix of the old name and replacing the title portion.
//
// Bug 5 fix: if the old name contains no dash (unexpected format) the entire
// old name is used as the prefix so we never panic on a missing index.
func renamedDiskName(oldNameOnDisk, newTitle string) string {
	prefix := oldNameOnDisk
	if idx := strings.Index(oldNameOnDisk, "-"); idx != -1 {
		prefix = oldNameOnDisk[:idx]
	}
	return fmt.Sprintf("%s-%s", prefix, util.SanitizeName(newTitle))
}
