package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"noti/internal/domain"
	"noti/internal/repository"
	"noti/internal/util"
)

// NoteService handles note business logic
type NoteService struct {
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	fileSystem    *repository.FileSystem
	notesPath     string
}

// NewNoteService creates a new note service
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

// GetAll returns all notes
func (s *NoteService) GetAll() ([]domain.Note, error) {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}
	return structure.Notes, nil
}

// Get returns a single note with its content
func (s *NoteService) Get(id string) (*domain.Note, error) {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}

	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			content, err := s.fileSystem.LoadNoteContent(&structure.Notes[i], structure)
			if err != nil {
				// If content fails to load, maybe the file is missing.
				// Return the note without content, but log the error.
				fmt.Printf("Warning: could not load content for note %s: %v\n", id, err)
				structure.Notes[i].Content = "" // Ensure content is empty
			} else {
				structure.Notes[i].Content = content
			}
			return &structure.Notes[i], nil
		}
	}

	return nil, fmt.Errorf("note not found")
}

// Create creates a new note
func (s *NoteService) Create(title string, content string, folderID string) (*domain.Note, error) {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %v", err)
	}

	if folderID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == folderID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("folder not found")
		}
	}

	now := time.Now()
	note := domain.Note{
		ID:         fmt.Sprintf("%d", now.UnixNano()),
		Title:      title,
		NameOnDisk: util.GenerateNameOnDisk(title) + ".md",
		FolderID:   folderID,
		Content:    content,
		CreatedAt:  now,
		UpdatedAt:  now,
		Order:      len(structure.Notes),
	}

	// Manually construct path for the new note and save it.
	parentPath := s.notesPath
	if folderID != "" {
		var err error
		parentPath, err = s.pathResolver.GetPathFor(folderID, structure)
		if err != nil {
			return nil, fmt.Errorf("could not resolve parent path for note: %v", err)
		}
	}
	notePath := filepath.Join(parentPath, note.NameOnDisk)
	if err := os.WriteFile(notePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to save new note content: %v", err)
	}

	structure.Notes = append(structure.Notes, note)
	if err := s.structureRepo.Save(structure); err != nil {
		return nil, fmt.Errorf("failed to save structure: %v", err)
	}

	return &note, nil
}

// Update updates an existing note
func (s *NoteService) Update(id string, title string, content string) error {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			// If title is different, rename the file on disk
			if structure.Notes[i].Title != title {
				oldPath, err := s.pathResolver.GetPathFor(id, structure)
				if err != nil {
					return fmt.Errorf("could not resolve old path for note update: %w", err)
				}

				parts := strings.SplitN(structure.Notes[i].NameOnDisk, "-", 2)
				timestamp := parts[0]
				newDiskName := fmt.Sprintf("%s-%s.md", timestamp, util.SanitizeName(title))

				parentPath := s.notesPath
				if structure.Notes[i].FolderID != "" {
					parentPath, err = s.pathResolver.GetPathFor(structure.Notes[i].FolderID, structure)
					if err != nil {
						return fmt.Errorf("could not resolve parent path for note update: %w", err)
					}
				}
				newPath := filepath.Join(parentPath, newDiskName)

				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("failed to rename note on disk: %w", err)
				}
				structure.Notes[i].NameOnDisk = newDiskName
			}

			structure.Notes[i].Title = title
			structure.Notes[i].UpdatedAt = time.Now()

			// Save the content to the (potentially new) path
			if err := s.fileSystem.SaveNoteContent(&structure.Notes[i], content, structure); err != nil {
				return err
			}

			return s.structureRepo.Save(structure)
		}
	}

	return fmt.Errorf("note not found")
}

// Move moves a note to a different folder
func (s *NoteService) Move(noteID string, targetFolderID string) error {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	if targetFolderID != "" {
		found := false
		for _, f := range structure.Folders {
			if f.ID == targetFolderID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("target folder not found")
		}
	}

	for i := range structure.Notes {
		if structure.Notes[i].ID == noteID {
			oldFolderID := structure.Notes[i].FolderID

			if err := s.moveNoteFile(&structure.Notes[i], oldFolderID, targetFolderID, structure); err != nil {
				return err
			}

			structure.Notes[i].FolderID = targetFolderID
			structure.Notes[i].UpdatedAt = time.Now()

			return s.structureRepo.Save(structure)
		}
	}

	return fmt.Errorf("note not found")
}

// Delete deletes a note
func (s *NoteService) Delete(id string) error {
	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	var noteToDelete *domain.Note
	for i := range structure.Notes {
		if structure.Notes[i].ID == id {
			noteToDelete = &structure.Notes[i]
			break
		}
	}

	if noteToDelete == nil {
		return fmt.Errorf("note not found")
	}

	// If deleting the file fails, abort the entire operation.
	if err := s.deleteNoteFile(noteToDelete, structure); err != nil {
		return fmt.Errorf("failed to delete note file from disk: %w", err)
	}

	newNotes := []domain.Note{}
	for _, note := range structure.Notes {
		if note.ID != id {
			newNotes = append(newNotes, note)
		}
	}
	structure.Notes = newNotes

	return s.structureRepo.Save(structure)
}

// moveNoteFile is an internal helper for moving note files
func (s *NoteService) moveNoteFile(note *domain.Note, oldFolderID string, newFolderID string, structure *domain.FolderStructure) error {
	return s.fileSystem.MoveNoteFile(note, oldFolderID, newFolderID, structure, s.notesPath)
}

// deleteNoteFile is an internal helper for deleting note files
func (s *NoteService) deleteNoteFile(note *domain.Note, structure *domain.FolderStructure) error {
	return s.fileSystem.DeleteNoteFile(note, structure)
}
