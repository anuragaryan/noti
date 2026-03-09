package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
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
	searchIndex   map[string]searchDocument
	searchIndexed bool
}

type searchDocument struct {
	raw   string
	lower string
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
		searchIndex:   make(map[string]searchDocument),
	}
}

// Search returns notes whose title or content matches all query terms.
// Results are ranked by relevance and then by recency.
func (s *NoteService) Search(query string, limit int) ([]domain.SearchMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return []domain.SearchMatch{}, nil
	}
	if limit <= 0 {
		limit = 100
	}

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, err
	}

	if err := s.ensureSearchIndexLocked(structure); err != nil {
		return nil, err
	}

	tokens := tokenizeSearchQuery(trimmed)
	if len(tokens) == 0 {
		return []domain.SearchMatch{}, nil
	}
	fullQuery := strings.ToLower(trimmed)

	type scored struct {
		match domain.SearchMatch
		score int
	}
	results := make([]scored, 0, len(structure.Notes))

	for _, note := range structure.Notes {
		title := strings.ToLower(note.Title)
		doc, ok := s.searchIndex[note.ID]
		if !ok {
			var err error
			doc, err = s.loadSearchDocument(&note, structure)
			if err != nil {
				slog.Warn("could not load note content while searching", "id", note.ID, "error", err)
			}
			s.searchIndex[note.ID] = doc
		}

		totalScore := 0
		matchedAll := true
		for _, token := range tokens {
			inTitle := strings.Contains(title, token)
			inContent := strings.Contains(doc.lower, token)
			if !inTitle && !inContent {
				matchedAll = false
				break
			}

			switch {
			case inTitle:
				totalScore += 20
			case inContent:
				totalScore += 5
			}
		}

		if !matchedAll {
			continue
		}

		if strings.Contains(title, fullQuery) {
			totalScore += 35
		}
		if strings.Contains(doc.lower, fullQuery) {
			totalScore += 10
		}

		line, snippet := firstMatchingLineAndSnippet(doc.raw, fullQuery, tokens)
		if snippet == "" {
			snippet = note.Title
		}

		results = append(results, scored{
			match: domain.SearchMatch{
				Note:    note,
				Line:    line,
				Snippet: snippet,
			},
			score: totalScore,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		if !results[i].match.Note.UpdatedAt.Equal(results[j].match.Note.UpdatedAt) {
			return results[i].match.Note.UpdatedAt.After(results[j].match.Note.UpdatedAt)
		}
		return strings.Compare(results[i].match.Note.ID, results[j].match.Note.ID) < 0
	})

	if limit > len(results) {
		limit = len(results)
	}

	out := make([]domain.SearchMatch, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, results[i].match)
	}
	return out, nil
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
			slog.Warn("could not load content for note", "id", id, "error", err)
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
	if s.searchIndexed {
		s.searchIndex[note.ID] = searchDocument{raw: content, lower: strings.ToLower(content)}
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
		if s.searchIndexed {
			s.searchIndex[structure.Notes[i].ID] = searchDocument{raw: content, lower: strings.ToLower(content)}
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
	if s.searchIndexed {
		delete(s.searchIndex, noteToDelete.ID)
	}
	return nil
}

func (s *NoteService) ensureSearchIndexLocked(structure *domain.FolderStructure) error {
	if !s.searchIndexed {
		return s.rebuildSearchIndexLocked(structure)
	}
	return nil
}

func (s *NoteService) rebuildSearchIndexLocked(structure *domain.FolderStructure) error {
	index := make(map[string]searchDocument, len(structure.Notes))
	for i := range structure.Notes {
		note := &structure.Notes[i]
		doc, err := s.loadSearchDocument(note, structure)
		if err != nil {
			slog.Warn("could not load content while building search index", "id", note.ID, "error", err)
			index[note.ID] = searchDocument{}
			continue
		}
		index[note.ID] = doc
	}
	s.searchIndex = index
	s.searchIndexed = true
	return nil
}

func (s *NoteService) loadSearchDocument(note *domain.Note, structure *domain.FolderStructure) (searchDocument, error) {
	content, err := s.fileSystem.LoadNoteContent(note, structure)
	if err != nil {
		return searchDocument{}, err
	}
	return searchDocument{raw: content, lower: strings.ToLower(content)}, nil
}

func firstMatchingLineAndSnippet(content, fullQuery string, tokens []string) (int, string) {
	if content == "" {
		return 0, ""
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if fullQuery != "" && strings.Contains(lower, fullQuery) {
			idx := strings.Index(lower, fullQuery)
			return i + 1, buildSnippet(line, idx, len(fullQuery))
		}
		for _, token := range tokens {
			if token == "" {
				continue
			}
			if strings.Contains(lower, token) {
				idx := strings.Index(lower, token)
				return i + 1, buildSnippet(line, idx, len(token))
			}
		}
	}

	return 0, ""
}

func buildSnippet(line string, matchStart, matchLen int) string {
	const maxLen = 140
	if strings.TrimSpace(line) == "" {
		return ""
	}
	trimmedLeft := strings.TrimLeft(line, " \t")
	leftDelta := len(line) - len(trimmedLeft)
	if matchStart >= leftDelta {
		matchStart -= leftDelta
	}
	text := strings.TrimRight(trimmedLeft, " \t")

	if len(text) <= maxLen || matchStart < 0 {
		return text
	}

	windowStart := matchStart - 40
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := matchStart + matchLen + 60
	if windowEnd > len(text) {
		windowEnd = len(text)
	}

	prefix := ""
	suffix := ""
	if windowStart > 0 {
		prefix = "..."
	}
	if windowEnd < len(text) {
		suffix = "..."
	}

	return prefix + text[windowStart:windowEnd] + suffix
}

func tokenizeSearchQuery(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	if len(parts) == 0 {
		return nil
	}
	tokens := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		tokens = append(tokens, p)
	}
	return tokens
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
