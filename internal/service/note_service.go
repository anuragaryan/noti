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

const splitStorageSchemaVersion = 2

// NoteService handles all note business logic.
// All exported methods are safe for concurrent use.
type NoteService struct {
	mu            sync.Mutex
	structureRepo *repository.StructureRepository
	pathResolver  *repository.PathResolver
	fileSystem    *repository.FileSystem
	searchIndex   map[string]searchDocument
	searchIndexed bool
}

type searchDocument struct {
	markdownRaw      string
	markdownLower    string
	transcriptRaw    string
	transcriptLower  string
	combinedLowerRaw string
}

// NewNoteService creates a new NoteService.
func NewNoteService(
	structureRepo *repository.StructureRepository,
	pathResolver *repository.PathResolver,
	fileSystem *repository.FileSystem,
	_ string,
) *NoteService {
	return &NoteService{
		structureRepo: structureRepo,
		pathResolver:  pathResolver,
		fileSystem:    fileSystem,
		searchIndex:   make(map[string]searchDocument),
	}
}

// EnsureSplitStorage validates and normalizes split markdown/transcript storage.
// This operation is idempotent.
func (s *NoteService) EnsureSplitStorage() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(s.pathResolver.MarkdownRootPath(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(s.pathResolver.TranscriptRootPath(), 0o755); err != nil {
		return err
	}

	mutated := false
	seenStems := make(map[string]struct{}, len(structure.Notes))

	for i := range structure.Notes {
		note := &structure.Notes[i]

		if strings.TrimSpace(note.FileStem) == "" {
			note.FileStem = deriveFileStem(note)
			mutated = true
		}
		note.FileStem = ensureUniqueFileStem(note.FileStem, seenStems)
		seenStems[note.FileStem] = struct{}{}

		markdownPath, err := s.pathResolver.NoteMarkdownPath(note, structure)
		if err != nil {
			return err
		}
		transcriptPath, err := s.pathResolver.NoteTranscriptPath(note, structure)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(markdownPath), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
			return err
		}

		if !fileExists(markdownPath) {
			if err := os.WriteFile(markdownPath, []byte(note.MarkdownContent), 0o644); err != nil {
				return err
			}
		}
		if !fileExists(transcriptPath) {
			if err := os.WriteFile(transcriptPath, []byte(note.TranscriptContent), 0o644); err != nil {
				return err
			}
		}

		mutated = true
	}

	if structure.SchemaVersion != splitStorageSchemaVersion {
		structure.SchemaVersion = splitStorageSchemaVersion
		mutated = true
	}

	if mutated {
		return s.structureRepo.Save(structure)
	}

	return nil
}

// Search returns notes whose title, markdown, or transcript matches all query terms.
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
			var loadErr error
			doc, loadErr = s.loadSearchDocument(&note, structure)
			if loadErr != nil {
				slog.Warn("could not load note content while searching", "id", note.ID, "error", loadErr)
			}
			s.searchIndex[note.ID] = doc
		}

		totalScore := 0
		matchedAll := true
		for _, token := range tokens {
			inTitle := strings.Contains(title, token)
			inMarkdown := strings.Contains(doc.markdownLower, token)
			inTranscript := strings.Contains(doc.transcriptLower, token)
			if !inTitle && !inMarkdown && !inTranscript {
				matchedAll = false
				break
			}

			switch {
			case inTitle:
				totalScore += 20
			case inMarkdown:
				totalScore += 8
			case inTranscript:
				totalScore += 5
			}
		}

		if !matchedAll {
			continue
		}

		if strings.Contains(title, fullQuery) {
			totalScore += 35
		}
		if strings.Contains(doc.markdownLower, fullQuery) {
			totalScore += 10
		}
		if strings.Contains(doc.transcriptLower, fullQuery) {
			totalScore += 7
		}

		line, snippet, source := bestSnippet(doc, note.Title, fullQuery, tokens)
		results = append(results, scored{
			match: domain.SearchMatch{
				Note:        note,
				Line:        line,
				Snippet:     snippet,
				SourceLabel: source,
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
		markdown, transcript, err := s.fileSystem.LoadNoteContentPair(&structure.Notes[i], structure)
		if err != nil {
			slog.Warn("could not load content for note", "id", id, "error", err)
		}
		structure.Notes[i].MarkdownContent = markdown
		structure.Notes[i].TranscriptContent = transcript
		return &structure.Notes[i], nil
	}

	return nil, fmt.Errorf("note %q not found", id)
}

// Create adds a new note.
func (s *NoteService) Create(title, markdownContent, folderID string) (*domain.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	structure, err := s.structureRepo.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load structure: %w", err)
	}

	if folderID != "" && !folderExists(folderID, structure) {
		return nil, fmt.Errorf("folder %q not found", folderID)
	}

	now := time.Now()
	fileStem := ensureUniqueFileStem(generateFileStem(title), currentFileStems(structure))
	note := domain.Note{
		ID:                  uuid.NewString(),
		Title:               title,
		FileStem:            fileStem,
		FolderID:            folderID,
		TranscriptActivated: false,
		MarkdownContent:     markdownContent,
		TranscriptContent:   "",
		CreatedAt:           now,
		UpdatedAt:           now,
		Order:               nextNoteOrder(structure),
	}

	if err := s.fileSystem.SaveNoteContentPair(&note, note.MarkdownContent, note.TranscriptContent, structure); err != nil {
		return nil, fmt.Errorf("failed to write note files: %w", err)
	}

	structure.Notes = append(structure.Notes, note)
	if err := s.structureRepo.Save(structure); err != nil {
		_ = s.fileSystem.DeleteNoteFilePair(&note, structure)
		return nil, fmt.Errorf("failed to save structure: %w", err)
	}
	if s.searchIndexed {
		s.searchIndex[note.ID] = buildSearchDocument(note.MarkdownContent, note.TranscriptContent)
	}

	return &note, nil
}

// Update updates title and note contents.
func (s *NoteService) Update(id, title, markdownContent, transcriptContent string) error {
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
			newStem := renamedFileStem(structure.Notes[i].FileStem, title)
			if fileStemTakenByAnotherNote(structure, id, newStem) {
				newStem = ensureUniqueFileStem(newStem, currentFileStems(structure))
			}
			if err := s.fileSystem.RenameNoteFilePair(&structure.Notes[i], newStem, structure); err != nil {
				return fmt.Errorf("failed to rename note files: %w", err)
			}
			structure.Notes[i].FileStem = newStem
		}

		structure.Notes[i].Title = title
		structure.Notes[i].UpdatedAt = time.Now()
		structure.Notes[i].MarkdownContent = markdownContent
		structure.Notes[i].TranscriptContent = transcriptContent

		if err := s.fileSystem.SaveNoteContentPair(&structure.Notes[i], markdownContent, transcriptContent, structure); err != nil {
			return fmt.Errorf("failed to save note contents: %w", err)
		}
		if s.searchIndexed {
			s.searchIndex[structure.Notes[i].ID] = buildSearchDocument(markdownContent, transcriptContent)
		}

		return s.structureRepo.Save(structure)
	}

	return fmt.Errorf("note %q not found", id)
}

// MarkTranscriptActivated enables transcript pane visibility for a note.
func (s *NoteService) MarkTranscriptActivated(id string) error {
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
		if structure.Notes[i].TranscriptActivated {
			return nil
		}
		structure.Notes[i].TranscriptActivated = true
		structure.Notes[i].UpdatedAt = time.Now()
		return s.structureRepo.Save(structure)
	}

	return fmt.Errorf("note %q not found", id)
}

// Move relocates a note to a different folder.
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

		if err := s.moveNoteFile(&structure.Notes[i], oldFolderID, targetFolderID, structure); err != nil {
			return err
		}

		structure.Notes[i].FolderID = targetFolderID
		structure.Notes[i].UpdatedAt = time.Now()

		if err := s.structureRepo.Save(structure); err != nil {
			_ = s.moveNoteFile(&structure.Notes[i], targetFolderID, oldFolderID, structure)
			return fmt.Errorf("failed to save structure after move: %w", err)
		}
		return nil
	}

	return fmt.Errorf("note %q not found", noteID)
}

// Delete removes a note files and metadata.
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
	if err := s.deleteNoteFile(&noteToDelete, structure); err != nil {
		return fmt.Errorf("failed to delete note files: %w", err)
	}

	structure.Notes = append(structure.Notes[:noteIndex], structure.Notes[noteIndex+1:]...)
	if err := s.structureRepo.Save(structure); err != nil {
		return fmt.Errorf("note files deleted but structure save failed (data may be inconsistent): %w", err)
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
	markdown, transcript, err := s.fileSystem.LoadNoteContentPair(note, structure)
	if err != nil {
		return searchDocument{}, err
	}
	return buildSearchDocument(markdown, transcript), nil
}

func buildSearchDocument(markdown, transcript string) searchDocument {
	return searchDocument{
		markdownRaw:      markdown,
		markdownLower:    strings.ToLower(markdown),
		transcriptRaw:    transcript,
		transcriptLower:  strings.ToLower(transcript),
		combinedLowerRaw: strings.ToLower(markdown + "\n" + transcript),
	}
}

func bestSnippet(doc searchDocument, title, fullQuery string, tokens []string) (int, string, string) {
	line, snippet := firstMatchingLineAndSnippet(doc.markdownRaw, fullQuery, tokens)
	markdownMatched := snippet != ""

	tLine, tSnippet := firstMatchingLineAndSnippet(doc.transcriptRaw, fullQuery, tokens)
	transcriptMatched := tSnippet != ""

	source := "Title"
	if markdownMatched && transcriptMatched {
		source = "Both"
	} else if markdownMatched {
		source = "Markdown"
	} else if transcriptMatched {
		source = "Transcript"
	}

	if markdownMatched {
		return line, snippet, source
	}
	if transcriptMatched {
		return tLine, tSnippet, source
	}

	if strings.TrimSpace(title) == "" {
		return 0, "", source
	}
	return 0, title, source
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

func (s *NoteService) moveNoteFile(note *domain.Note, oldFolderID, newFolderID string, structure *domain.FolderStructure) error {
	return s.fileSystem.MoveNoteFilePair(note, oldFolderID, newFolderID, structure)
}

func (s *NoteService) deleteNoteFile(note *domain.Note, structure *domain.FolderStructure) error {
	return s.fileSystem.DeleteNoteFilePair(note, structure)
}

func folderExists(id string, structure *domain.FolderStructure) bool {
	for _, f := range structure.Folders {
		if f.ID == id {
			return true
		}
	}
	return false
}

func nextNoteOrder(structure *domain.FolderStructure) int {
	max := -1
	for _, n := range structure.Notes {
		if n.Order > max {
			max = n.Order
		}
	}
	return max + 1
}

func generateFileStem(title string) string {
	return fmt.Sprintf("%d-%s-%s", time.Now().Unix(), uuid.NewString()[:8], util.SanitizeName(title))
}

func deriveFileStem(note *domain.Note) string {
	if strings.TrimSpace(note.FileStem) != "" {
		return note.FileStem
	}
	return generateFileStem(note.Title)
}

func currentFileStems(structure *domain.FolderStructure) map[string]struct{} {
	stems := make(map[string]struct{}, len(structure.Notes))
	for _, note := range structure.Notes {
		if strings.TrimSpace(note.FileStem) == "" {
			continue
		}
		stems[note.FileStem] = struct{}{}
	}
	return stems
}

func ensureUniqueFileStem(stem string, existing map[string]struct{}) string {
	candidate := strings.TrimSpace(stem)
	if candidate == "" {
		candidate = uuid.NewString()
	}
	if _, exists := existing[candidate]; !exists {
		return candidate
	}
	for i := 1; ; i++ {
		next := fmt.Sprintf("%s-%d", candidate, i)
		if _, exists := existing[next]; !exists {
			return next
		}
	}
}

func fileStemTakenByAnotherNote(structure *domain.FolderStructure, noteID, fileStem string) bool {
	for _, note := range structure.Notes {
		if note.ID == noteID {
			continue
		}
		if note.FileStem == fileStem {
			return true
		}
	}
	return false
}

func renamedFileStem(oldFileStem, newTitle string) string {
	base := strings.TrimSpace(oldFileStem)
	newName := util.SanitizeName(newTitle)
	if base == "" {
		return generateFileStem(newTitle)
	}
	parts := strings.SplitN(base, "-", 3)
	if len(parts) < 2 {
		return fmt.Sprintf("%s-%s", base, newName)
	}
	return fmt.Sprintf("%s-%s-%s", parts[0], parts[1], newName)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
