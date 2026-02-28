package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/util"
)

// FolderService handles all folder business logic.
// All exported methods are safe for concurrent use.
type FolderService struct {
	mu            sync.Mutex
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	notesPath     string
}

// NewFolderService creates a new FolderService.
func NewFolderService(
	structureRepo *repository.StructureRepository,
	pathResolver *repository.PathResolver,
	notesPath string,
) *FolderService {
	return &FolderService{
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		notesPath:     notesPath,
	}
}

// GetAll returns all folders.
func (s *FolderService) GetAll() ([]domain.Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}
	return structure.Folders, nil
}

// Create creates a new folder on disk and records it in the structure.
func (s *FolderService) Create(name, parentID string) (*domain.Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %w", err)
	}

	if parentID != "" && !folderExists(parentID, structure) {
		return nil, fmt.Errorf("parent folder %q not found", parentID)
	}

	now := time.Now()
	folder := domain.Folder{
		// Use UUID for the same reason as notes: UnixNano can collide.
		ID:         "f_" + uuid.NewString(),
		Name:       name,
		NameOnDisk: util.GenerateNameOnDisk(name),
		ParentID:   parentID,
		CreatedAt:  now,
		// Bug 7 fix: use max+1 so deletions never produce duplicate order values.
		Order: nextFolderOrder(structure),
	}

	parentPath := s.notesPath
	if parentID != "" {
		parentPath, err = s.pathResolver.GetPathFor(parentID, structure)
		if err != nil {
			return nil, fmt.Errorf("could not resolve parent path: %w", err)
		}
	}

	newFolderPath := filepath.Join(parentPath, folder.NameOnDisk)
	if err := os.MkdirAll(newFolderPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create folder directory: %w", err)
	}

	structure.Folders = append(structure.Folders, folder)
	if err := s.structureRepo.Save(structure); err != nil {
		// Roll back the directory so we do not leave an orphan on disk.
		_ = os.Remove(newFolderPath)
		return nil, fmt.Errorf("failed to save structure: %w", err)
	}

	return &folder, nil
}

// Update renames a folder and, when the parentID changes, physically moves the
// directory to its new location on disk.
func (s *FolderService) Update(id, name, parentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	// Bug 4 fix: validate the move whenever parentID changes, including when
	// the folder is being moved to the root (parentID == "").
	for _, f := range structure.Folders {
		if f.ID != id {
			continue
		}
		if parentID != f.ParentID {
			if err := s.validateMove(id, parentID, structure); err != nil {
				return err
			}
		}
		break
	}

	for i := range structure.Folders {
		if structure.Folders[i].ID != id {
			continue
		}

		currentParentID := structure.Folders[i].ParentID
		currentName := structure.Folders[i].Name

		// Determine whether a rename or a parent move (or both) is needed.
		nameChanged := currentName != name
		parentChanged := currentParentID != parentID

		if nameChanged || parentChanged {
			// We need the old absolute path before touching the struct.
			oldPath, err := s.pathResolver.GetPathFor(id, structure)
			if err != nil {
				return fmt.Errorf("could not resolve current path for folder %q: %w", id, err)
			}

			// Compute the new NameOnDisk.
			newDiskName := structure.Folders[i].NameOnDisk
			if nameChanged {
				// Bug 5 fix: safe prefix extraction.
				newDiskName = renamedDiskName(structure.Folders[i].NameOnDisk, name)
			}

			// Resolve the new parent directory.
			newParentPath := s.notesPath
			targetParentID := parentID
			if !parentChanged {
				targetParentID = currentParentID
			}
			if targetParentID != "" {
				newParentPath, err = s.pathResolver.GetPathFor(targetParentID, structure)
				if err != nil {
					return fmt.Errorf("could not resolve new parent path for folder %q: %w", id, err)
				}
			}

			newPath := filepath.Join(newParentPath, newDiskName)

			// Bug 2 fix: actually move the directory on disk when anything changes.
			if oldPath != newPath {
				if err := os.MkdirAll(newParentPath, 0755); err != nil {
					return fmt.Errorf("could not create new parent directory: %w", err)
				}
				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("failed to move/rename folder on disk: %w", err)
				}
			}

			structure.Folders[i].NameOnDisk = newDiskName
		}

		structure.Folders[i].Name = name
		structure.Folders[i].ParentID = parentID

		return s.structureRepo.Save(structure)
	}

	return fmt.Errorf("folder %q not found", id)
}

// Delete removes a folder. When deleteNotes is true every note inside is
// deleted from disk and removed from the structure. When false the notes are
// moved to the root first. The folder directory is removed last so that all
// note files are out of it before it disappears.
func (s *FolderService) Delete(id string, deleteNotes bool, noteService *NoteService) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	// Capture the folder path before we touch the structure so path resolution
	// still works (the folder is still in the slice at this point).
	folderPath, pathErr := s.pathResolver.GetPathFor(id, structure)
	if pathErr != nil {
		return fmt.Errorf("cannot resolve path for folder %q: %w", id, pathErr)
	}

	// Refuse to delete a folder that still has sub-folders.
	for _, f := range structure.Folders {
		if f.ParentID == id {
			return fmt.Errorf("cannot delete folder %q: it still contains sub-folders", id)
		}
	}

	if deleteNotes {
		// Bug 11 fix: collect errors from individual note file deletions and
		// return a combined error instead of silently swallowing them.
		var deleteErrors []error
		var remainingNotes []domain.Note

		for i := range structure.Notes {
			if structure.Notes[i].FolderID != id {
				remainingNotes = append(remainingNotes, structure.Notes[i])
				continue
			}
			if err := noteService.deleteNoteFile(&structure.Notes[i], structure); err != nil {
				deleteErrors = append(deleteErrors, fmt.Errorf("note %q: %w", structure.Notes[i].ID, err))
			}
		}

		if len(deleteErrors) > 0 {
			return fmt.Errorf("failed to delete some note files in folder %q: %v", id, deleteErrors)
		}
		structure.Notes = remainingNotes

	} else {
		// Bug 8 fix: move all notes out first. If any move fails, return an
		// error immediately — the caller can retry. Notes that were already
		// moved will be on disk at the root but their FolderID in the
		// (unsaved) structure still points at the old folder, so a retry will
		// see them in the old location and attempt the move again.
		for i := range structure.Notes {
			if structure.Notes[i].FolderID != id {
				continue
			}
			if err := noteService.moveNoteFile(&structure.Notes[i], id, "", structure); err != nil {
				return fmt.Errorf("failed to move note %q to root during folder delete: %w", structure.Notes[i].ID, err)
			}
			structure.Notes[i].FolderID = ""
		}
	}

	// Remove the folder from the structure slice.
	newFolders := make([]domain.Folder, 0, len(structure.Folders)-1)
	for _, f := range structure.Folders {
		if f.ID != id {
			newFolders = append(newFolders, f)
		}
	}
	structure.Folders = newFolders

	// Persist the updated structure before removing the directory so that even
	// if RemoveAll fails the structure is correct.
	if err := s.structureRepo.Save(structure); err != nil {
		return fmt.Errorf("failed to save structure during folder delete: %w", err)
	}

	// Finally, remove the (now-empty) directory from disk.
	if err := os.RemoveAll(folderPath); err != nil {
		return fmt.Errorf("failed to remove folder directory %q: %w", folderPath, err)
	}

	return nil
}

// GetPath returns the ancestor chain for a folder as a breadcrumb trail,
// starting from the top-level folder down to the given folder.
func (s *FolderService) GetPath(folderID string) ([]domain.Folder, error) {
	if folderID == "" {
		return []domain.Folder{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}

	var path []domain.Folder
	currentID := folderID

	for currentID != "" {
		found := false
		for _, folder := range structure.Folders {
			if folder.ID != currentID {
				continue
			}
			// Prepend so the slice ends up in root→leaf order.
			path = append([]domain.Folder{folder}, path...)
			currentID = folder.ParentID
			found = true
			break
		}
		if !found {
			break
		}
	}

	return path, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// nextFolderOrder returns one past the current maximum folder order so that
// deletions never create duplicate order values.
func nextFolderOrder(structure *domain.FolderStructure) int {
	max := -1
	for _, f := range structure.Folders {
		if f.Order > max {
			max = f.Order
		}
	}
	return max + 1
}

// validateMove verifies that placing folderID under newParentID would not
// create a circular reference in the folder tree.
//
// Bug 4 fix: this is now called whenever parentID changes, including when the
// new parent is "" (root), to keep the validation logic in one place.
func (s *FolderService) validateMove(folderID, newParentID string, structure *domain.FolderStructure) error {
	if folderID == newParentID {
		return fmt.Errorf("cannot move folder %q into itself", folderID)
	}

	if newParentID == "" {
		// Moving to root is always valid (no circularity possible).
		return nil
	}

	if !folderExists(newParentID, structure) {
		return fmt.Errorf("parent folder %q not found", newParentID)
	}

	// Walk up from newParentID; if we ever hit folderID the move would create
	// a cycle.
	currentID := newParentID
	for currentID != "" {
		if currentID == folderID {
			return fmt.Errorf("cannot move folder %q: would create a circular reference", folderID)
		}
		found := false
		for _, f := range structure.Folders {
			if f.ID == currentID {
				currentID = f.ParentID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	return nil
}
