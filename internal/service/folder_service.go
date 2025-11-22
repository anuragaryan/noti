package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"noti/internal/domain"
	"noti/internal/repository"
)

// FolderService handles folder business logic
type FolderService struct {
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	notesPath     string
}

// NewFolderService creates a new folder service
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

// GetAll returns all folders
func (s *FolderService) GetAll() ([]domain.Folder, error) {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}
	return structure.Folders, nil
}

// Create creates a new folder
func (s *FolderService) Create(name string, parentID string) (*domain.Folder, error) {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %v", err)
	}

	if parentID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == parentID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("parent folder not found")
		}
	}

	now := time.Now()
	folder := domain.Folder{
		ID:         fmt.Sprintf("f_%d", now.UnixNano()),
		Name:       name,
		NameOnDisk: generateNameOnDisk(name),
		ParentID:   parentID,
		CreatedAt:  now,
		Order:      len(structure.Folders),
	}

	parentPath := s.notesPath
	if parentID != "" {
		var err error
		parentPath, err = s.pathResolver.GetPathFor(parentID, structure)
		if err != nil {
			return nil, fmt.Errorf("could not resolve parent path: %v", err)
		}
	}

	newFolderPath := filepath.Join(parentPath, folder.NameOnDisk)
	if err := os.MkdirAll(newFolderPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create folder directory: %v", err)
	}

	structure.Folders = append(structure.Folders, folder)
	if err := s.structureRepo.Save(structure); err != nil {
		return nil, fmt.Errorf("failed to save structure: %v", err)
	}

	return &folder, nil
}

// Update updates an existing folder
func (s *FolderService) Update(id string, name string, parentID string) error {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	if parentID != "" && parentID != id {
		if err := s.validateMove(id, parentID, structure); err != nil {
			return err
		}
	}

	for i := range structure.Folders {
		if structure.Folders[i].ID == id {
			// If the name is different, we need to rename the folder on disk
			if structure.Folders[i].Name != name {
				oldPath, err := s.pathResolver.GetPathFor(id, structure)
				if err != nil {
					return fmt.Errorf("could not resolve old path for folder rename: %w", err)
				}

				// Keep the original timestamp, just update the name part
				parts := strings.SplitN(structure.Folders[i].NameOnDisk, "-", 2)
				timestamp := parts[0]
				newDiskName := fmt.Sprintf("%s-%s", timestamp, SanitizeName(name))

				parentPath := s.notesPath
				if structure.Folders[i].ParentID != "" {
					parentPath, err = s.pathResolver.GetPathFor(structure.Folders[i].ParentID, structure)
					if err != nil {
						return fmt.Errorf("could not resolve parent path for rename: %w", err)
					}
				}
				newPath := filepath.Join(parentPath, newDiskName)

				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("failed to rename folder on disk: %w", err)
				}
				structure.Folders[i].NameOnDisk = newDiskName
			}

			structure.Folders[i].Name = name
			if parentID != structure.Folders[i].ParentID {
				// Moving folders is not implemented in this refactor, but we keep the check
				structure.Folders[i].ParentID = parentID
			}
			return s.structureRepo.Save(structure)
		}
	}

	return fmt.Errorf("folder not found")
}

// Delete deletes a folder
func (s *FolderService) Delete(id string, deleteNotes bool, noteService *NoteService) error {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	// Find the folder to get its path BEFORE removing it from the structure
	folderPath, err := s.pathResolver.GetPathFor(id, structure)
	if err != nil {
		// If we can't get the path, we can't delete it. Log it and continue.
		fmt.Printf("Warning: could not resolve path for folder %s to delete it from disk: %v\n", id, err)
	}

	for _, folder := range structure.Folders {
		if folder.ParentID == id {
			return fmt.Errorf("cannot delete folder with subfolders")
		}
	}

	if deleteNotes {
		var notesInFolder []*domain.Note
		var remainingNotes []domain.Note
		for i := range structure.Notes {
			if structure.Notes[i].FolderID == id {
				notesInFolder = append(notesInFolder, &structure.Notes[i])
			} else {
				remainingNotes = append(remainingNotes, structure.Notes[i])
			}
		}

		for _, note := range notesInFolder {
			if err := noteService.deleteNoteFile(note, structure); err != nil {
				// Log error but continue trying to delete other notes
				fmt.Printf("Warning: failed to delete note file %s: %v\n", note.ID, err)
			}
		}
		structure.Notes = remainingNotes

	} else {
		// Move notes to root
		for i := range structure.Notes {
			if structure.Notes[i].FolderID == id {
				if err := noteService.moveNoteFile(&structure.Notes[i], id, "", structure); err != nil {
					return fmt.Errorf("failed to move note out of deleted folder: %w", err)
				}
				structure.Notes[i].FolderID = ""
			}
		}
	}

	// Now remove the folder from the structure
	newFolders := []domain.Folder{}
	for _, folder := range structure.Folders {
		if folder.ID != id {
			newFolders = append(newFolders, folder)
		}
	}
	structure.Folders = newFolders

	// And finally, remove the folder from disk if the path was found
	if err == nil { // Only try to remove if GetPathFor succeeded
		if err := os.RemoveAll(folderPath); err != nil {
			return fmt.Errorf("failed to delete folder directory '%s': %w", folderPath, err)
		}
	}

	return s.structureRepo.Save(structure)
}

// GetPath returns the folder path (breadcrumb trail)
func (s *FolderService) GetPath(folderID string) ([]domain.Folder, error) {
	if folderID == "" {
		return []domain.Folder{}, nil
	}

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}

	path := []domain.Folder{}
	currentID := folderID

	for currentID != "" {
		found := false
		for _, folder := range structure.Folders {
			if folder.ID == currentID {
				path = append([]domain.Folder{folder}, path...)
				currentID = folder.ParentID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	return path, nil
}

// validateMove validates that a folder move is valid (no circular references)
func (s *FolderService) validateMove(folderID string, newParentID string, structure *domain.FolderStructure) error {
	if folderID == newParentID {
		return fmt.Errorf("cannot move folder into itself")
	}

	currentID := newParentID
	for currentID != "" {
		if currentID == folderID {
			return fmt.Errorf("cannot create circular folder reference")
		}

		found := false
		for _, folder := range structure.Folders {
			if folder.ID == currentID {
				currentID = folder.ParentID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	if newParentID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == newParentID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("parent folder not found")
		}
	}

	return nil
}

// SanitizeName removes characters that are problematic for file systems
func SanitizeName(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ToLower(name)
	// Basic sanitization
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		return "untitled"
	}
	return name
}

// generateNameOnDisk creates a filesystem-friendly name with a timestamp
func generateNameOnDisk(name string) string {
	now := time.Now().Unix()
	return fmt.Sprintf("%d-%s", now, SanitizeName(name))
}
