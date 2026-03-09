package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"noti/internal/domain"
	"noti/internal/repository"
)

// noteServiceHarness wires up a NoteService backed by a real temp directory.
type noteServiceHarness struct {
	notesPath     string
	structureRepo *repository.StructureRepository
	service       *NoteService
	folderService *FolderService
}

func newNoteServiceHarness(t *testing.T) *noteServiceHarness {
	t.Helper()
	notesPath := t.TempDir()

	structurePath := filepath.Join(notesPath, "structure.json")
	structureRepo := repository.NewStructureRepository(structurePath)
	pathResolver := repository.NewPathResolver(notesPath)
	fs := repository.NewFileSystem(pathResolver)
	svc := NewNoteService(structureRepo, pathResolver, fs, notesPath)
	folderSvc := NewFolderService(structureRepo, pathResolver, notesPath)

	if err := structureRepo.Save(&domain.FolderStructure{
		Folders: []domain.Folder{},
		Notes:   []domain.Note{},
	}); err != nil {
		t.Fatalf("harness setup: could not save initial structure: %v", err)
	}

	return &noteServiceHarness{
		notesPath:     notesPath,
		structureRepo: structureRepo,
		service:       svc,
		folderService: folderSvc,
	}
}

// createFolder creates a root-level folder via the public FolderService API.
func (h *noteServiceHarness) createFolder(t *testing.T, name string) *domain.Folder {
	t.Helper()
	folder, err := h.folderService.Create(name, "")
	if err != nil {
		t.Fatalf("createFolder %q: %v", name, err)
	}
	return folder
}

// ── Create ────────────────────────────────────────────────────────────────────

// TestNoteService_Create_CreatesFileAndStructureEntry verifies that Create writes
// both the file to disk and the note entry into the persisted structure.
func TestNoteService_Create_CreatesFileAndStructureEntry(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, err := h.service.Create("Hello", "world content", "")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if note.ID == "" {
		t.Error("note.ID should not be empty")
	}

	// File must exist on disk with correct content.
	filePath := filepath.Join(h.notesPath, note.NameOnDisk)
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("note file should exist on disk: %v", err)
	}
	if string(data) != "world content" {
		t.Errorf("file content: got %q, want %q", string(data), "world content")
	}

	// Verify structure persistence via GetAll, not just the returned value.
	notes, err := h.service.GetAll()
	if err != nil {
		t.Fatalf("GetAll after Create: %v", err)
	}
	found := false
	for _, n := range notes {
		if n.ID == note.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created note should appear in GetAll, confirming structure was saved")
	}
}

func TestNoteService_Create_UnknownFolder_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	_, err := h.service.Create("Note", "content", "nonexistent-folder-id")
	if err == nil {
		t.Fatal("expected error for unknown folder, got nil")
	}
}

// TestNoteService_Create_OrderNeverDuplicatesAfterDeletion verifies that no two
// active notes ever share the same Order value, even after a deletion.
func TestNoteService_Create_OrderNeverDuplicatesAfterDeletion(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	n1, err := h.service.Create("First", "", "")
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	n2, err := h.service.Create("Second", "", "")
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if err := h.service.Delete(n2.ID); err != nil {
		t.Fatalf("Delete second: %v", err)
	}
	n3, err := h.service.Create("Third", "", "")
	if err != nil {
		t.Fatalf("Create third: %v", err)
	}

	// No two active notes may share the same Order; n3 must not collide with n1.
	if n3.Order == n1.Order {
		t.Errorf("n3.Order (%d) must not equal n1.Order (%d): active notes have duplicate orders", n3.Order, n1.Order)
	}
}

// TestNoteService_Create_IDsAreUnique verifies that every Create call returns
// a distinct ID, even under rapid sequential creation.
func TestNoteService_Create_IDsAreUnique(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		// Use distinct titles so NameOnDisk never collides.
		title := strings.Repeat("x", i+1)
		note, err := h.service.Create(title, "", "")
		if err != nil {
			t.Fatalf("Create[%d]: unexpected error: %v", i, err)
		}
		if seen[note.ID] {
			t.Errorf("duplicate note ID produced: %q", note.ID)
		}
		seen[note.ID] = true
	}
}

func TestNoteService_Create_InsideFolder_PlacesFileInFolderDirectory(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)
	folder := h.createFolder(t, "work")

	note, err := h.service.Create("Plan", "plan content", folder.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	filePath := filepath.Join(h.notesPath, folder.NameOnDisk, note.NameOnDisk)
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("note file should be inside folder directory: %v", err)
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestNoteService_Get_ReturnsNoteWithContent(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	created, _ := h.service.Create("My Note", "the content", "")
	got, err := h.service.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if got.Content != "the content" {
		t.Errorf("content: got %q, want %q", got.Content, "the content")
	}
}

func TestNoteService_Get_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	_, err := h.service.Get("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown note ID, got nil")
	}
}

// TestNoteService_Get_FileMissingOnDisk_ReturnsNoteWithEmptyContent verifies that
// Get succeeds even when the note's file is absent from disk, returning the note
// with empty content so the UI can still display its metadata.
func TestNoteService_Get_FileMissingOnDisk_ReturnsNoteWithEmptyContent(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, err := h.service.Create("Existing Note", "content", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Remove the file from disk to simulate data loss / manual deletion.
	if err := os.Remove(filepath.Join(h.notesPath, note.NameOnDisk)); err != nil {
		t.Fatalf("setup: could not remove file: %v", err)
	}

	// Get must still succeed and return the note, just with empty content.
	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get with missing file: expected success, got error: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil note")
	}
	if got.Content != "" {
		t.Errorf("expected empty content for file-missing note, got %q", got.Content)
	}
	if got.ID != note.ID {
		t.Errorf("note ID: got %q, want %q", got.ID, note.ID)
	}
}

// ── GetAll ────────────────────────────────────────────────────────────────────

func TestNoteService_GetAll_ReturnsAllNotes(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	for _, title := range []string{"A", "B", "C"} {
		if _, err := h.service.Create(title, "", ""); err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
	}

	notes, err := h.service.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(notes) != 3 {
		t.Errorf("expected 3 notes, got %d", len(notes))
	}
}

func TestNoteService_Search_FindsContentMatches(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	n, err := h.service.Create("Meeting", "Discuss roadmap and launch timeline", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	results, err := h.service.Search("launch", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Note.ID != n.ID {
		t.Fatalf("expected note %q, got %q", n.ID, results[0].Note.ID)
	}
	if results[0].Line <= 0 {
		t.Fatalf("expected positive line number, got %d", results[0].Line)
	}
}

func TestNoteService_Search_UsesTokenizedANDMatching(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	both, err := h.service.Create("Alpha Beta", "combined terms", "")
	if err != nil {
		t.Fatalf("Create both: %v", err)
	}
	if _, err := h.service.Create("Only Alpha", "single term", ""); err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	if _, err := h.service.Create("Only Beta", "single term", ""); err != nil {
		t.Fatalf("Create beta: %v", err)
	}

	results, err := h.service.Search("alpha beta", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 AND result, got %d", len(results))
	}
	if results[0].Note.ID != both.ID {
		t.Fatalf("expected note %q, got %q", both.ID, results[0].Note.ID)
	}
}

func TestNoteService_Search_PrioritizesTitleMatches(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	titleHit, err := h.service.Create("Roadmap Plan", "misc", "")
	if err != nil {
		t.Fatalf("Create title hit: %v", err)
	}
	if _, err := h.service.Create("Notes", "contains roadmap in content", ""); err != nil {
		t.Fatalf("Create content hit: %v", err)
	}

	results, err := h.service.Search("roadmap", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].Note.ID != titleHit.ID {
		t.Fatalf("expected title hit %q first, got %q", titleHit.ID, results[0].Note.ID)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestNoteService_Update_ChangesContentOnDisk(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, _ := h.service.Create("Original", "old content", "")
	if err := h.service.Update(note.ID, "Original", "new content"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := h.service.Get(note.ID)
	if got.Content != "new content" {
		t.Errorf("content after update: got %q, want %q", got.Content, "new content")
	}
}

func TestNoteService_Update_TitleChange_RenamesFileOnDisk(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, _ := h.service.Create("Old Title", "content", "")
	oldDiskName := note.NameOnDisk

	if err := h.service.Update(note.ID, "New Title", "content"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Old file must be gone.
	if _, err := os.Stat(filepath.Join(h.notesPath, oldDiskName)); !os.IsNotExist(err) {
		t.Error("old file should have been removed after rename")
	}

	// New file must be readable via Get.
	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get after rename: %v", err)
	}
	if got.Title != "New Title" {
		t.Errorf("title: got %q, want %q", got.Title, "New Title")
	}
}

// TestNoteService_Update_TitleChange_InsideFolder_RenamesFileCorrectly verifies
// that renaming a note inside a folder updates the file at the correct path.
func TestNoteService_Update_TitleChange_InsideFolder_RenamesFileCorrectly(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)
	folder := h.createFolder(t, "archive")

	note, err := h.service.Create("Draft", "content", folder.ID)
	if err != nil {
		t.Fatalf("Create in folder: %v", err)
	}
	oldDiskName := note.NameOnDisk

	if err := h.service.Update(note.ID, "Final", "content"); err != nil {
		t.Fatalf("Update inside folder: %v", err)
	}

	// Old file inside the folder must be gone.
	oldPath := filepath.Join(h.notesPath, folder.NameOnDisk, oldDiskName)
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old file inside folder should have been removed after rename")
	}

	// Get must succeed and the new file must be inside the folder.
	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get after rename inside folder: %v", err)
	}
	if got.Title != "Final" {
		t.Errorf("title: got %q, want %q", got.Title, "Final")
	}
	newPath := filepath.Join(h.notesPath, folder.NameOnDisk, got.NameOnDisk)
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("renamed file should exist inside folder: %v", err)
	}
}

// TestNoteService_Update_NameOnDiskWithNoDash_DoesNotPanic verifies that Update
// handles a NameOnDisk that contains no dash without panicking.
func TestNoteService_Update_NameOnDiskWithNoDash_DoesNotPanic(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	// Create a note normally, then manually overwrite its NameOnDisk with a
	// no-dash name directly on disk and in the structure.
	note, err := h.service.Create("Original", "content", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Rename the file on disk to have no dash.
	oldPath := filepath.Join(h.notesPath, note.NameOnDisk)
	noDashPath := filepath.Join(h.notesPath, "nodash.md")
	if err := os.Rename(oldPath, noDashPath); err != nil {
		t.Fatalf("setup rename: %v", err)
	}

	// Update the structure entry to reflect the no-dash name.
	structure, err := h.structureRepo.Load()
	if err != nil {
		t.Fatalf("setup load: %v", err)
	}
	for i := range structure.Notes {
		if structure.Notes[i].ID == note.ID {
			structure.Notes[i].NameOnDisk = "nodash.md"
		}
	}
	if err := h.structureRepo.Save(structure); err != nil {
		t.Fatalf("setup save: %v", err)
	}

	// Must not panic, and must succeed.
	if err := h.service.Update(note.ID, "New Name", "content"); err != nil {
		t.Fatalf("Update with no-dash NameOnDisk: %v", err)
	}
}

func TestNoteService_Update_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	err := h.service.Update("ghost", "Title", "Content")
	if err == nil {
		t.Fatal("expected error for unknown note ID, got nil")
	}
}

// ── Move ──────────────────────────────────────────────────────────────────────

func TestNoteService_Move_ToFolder_MovesFileOnDisk(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)
	folder := h.createFolder(t, "archive")

	note, _ := h.service.Create("Note", "content", "")
	if err := h.service.Move(note.ID, folder.ID); err != nil {
		t.Fatalf("Move: %v", err)
	}

	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get after move: %v", err)
	}
	if got.FolderID != folder.ID {
		t.Errorf("FolderID: got %q, want %q", got.FolderID, folder.ID)
	}
}

// TestNoteService_Move_ToRoot_MovesFileOnDisk verifies both the structure entry
// and the physical file location after moving a note back to the root.
func TestNoteService_Move_ToRoot_MovesFileOnDisk(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)
	folder := h.createFolder(t, "inbox")

	note, _ := h.service.Create("InFolder", "content", folder.ID)
	if err := h.service.Move(note.ID, ""); err != nil {
		t.Fatalf("Move to root: %v", err)
	}

	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get after move to root: %v", err)
	}
	if got.FolderID != "" {
		t.Errorf("FolderID should be empty after move to root, got %q", got.FolderID)
	}

	// Verify the file is physically at root, not still inside the old folder.
	rootFilePath := filepath.Join(h.notesPath, note.NameOnDisk)
	if _, err := os.Stat(rootFilePath); err != nil {
		t.Errorf("file should exist at notes root after move: %v", err)
	}
	folderFilePath := filepath.Join(h.notesPath, folder.NameOnDisk, note.NameOnDisk)
	if _, err := os.Stat(folderFilePath); !os.IsNotExist(err) {
		t.Error("file should no longer exist inside the old folder directory")
	}
}

func TestNoteService_Move_UnknownFolder_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, _ := h.service.Create("Note", "content", "")
	err := h.service.Move(note.ID, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent target folder, got nil")
	}
}

func TestNoteService_Move_UnknownNote_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	err := h.service.Move("ghost", "")
	if err == nil {
		t.Fatal("expected error for unknown note, got nil")
	}
}

// TestNoteService_Move_RollsBackFileOnSaveFailure simulates a Save failure after
// the file has already moved on disk and verifies the file is rolled back.
func TestNoteService_Move_RollsBackFileOnSaveFailure(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)
	folder := h.createFolder(t, "dest")

	note, err := h.service.Create("Note", "content", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Make the structure file read-only so Save will fail.
	structurePath := filepath.Join(h.notesPath, "structure.json")
	if err := os.Chmod(structurePath, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	// Restore permissions when the test ends so TempDir cleanup can remove it.
	t.Cleanup(func() { os.Chmod(structurePath, 0644) })

	err = h.service.Move(note.ID, folder.ID)
	if err == nil {
		t.Fatal("expected error when structure save fails, got nil")
	}

	// Restore write permission to check the file location.
	os.Chmod(structurePath, 0644)

	// After rollback the file must be back at its original location (root),
	// not inside the destination folder.
	originalPath := filepath.Join(h.notesPath, note.NameOnDisk)
	if _, err := os.Stat(originalPath); err != nil {
		t.Errorf("file should have been rolled back to original location: %v", err)
	}
	movedPath := filepath.Join(h.notesPath, folder.NameOnDisk, note.NameOnDisk)
	if _, err := os.Stat(movedPath); !os.IsNotExist(err) {
		t.Error("file should not remain in destination after rollback")
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestNoteService_Delete_RemovesFileAndStructureEntry(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, _ := h.service.Create("ToDelete", "content", "")
	filePath := filepath.Join(h.notesPath, note.NameOnDisk)

	if err := h.service.Delete(note.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should have been deleted from disk")
	}

	notes, _ := h.service.GetAll()
	for _, n := range notes {
		if n.ID == note.ID {
			t.Error("deleted note should not appear in GetAll")
		}
	}
}

func TestNoteService_Delete_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	err := h.service.Delete("ghost")
	if err == nil {
		t.Fatal("expected error for unknown note ID, got nil")
	}
}

// ── Concurrency ───────────────────────────────────────────────────────────────

func TestNoteService_ConcurrentCreates_NoDuplicateIDs(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	const goroutines = 10
	type result struct {
		id  string
		err error
	}
	results := make(chan result, goroutines)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Use distinct titles so NameOnDisk never collides.
			note, err := h.service.Create(strings.Repeat("t", i+1), "", "")
			if err != nil {
				results <- result{err: err}
				return
			}
			results <- result{id: note.ID}
		}(i)
	}

	wg.Wait()
	close(results)

	seen := map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Errorf("concurrent Create error: %v", r.err)
			continue
		}
		if seen[r.id] {
			t.Errorf("duplicate ID from concurrent Create: %q", r.id)
		}
		seen[r.id] = true
	}
}

// ── internal helpers used by tests ────────────────────────────────────────────

// plantNoteInStructure inserts a note directly into the persisted structure
// and writes its file to disk. Used when we need fine-grained control over
// the note's NameOnDisk (e.g. to test edge cases like no-dash filenames).
func (h *noteServiceHarness) plantNoteInStructure(t *testing.T, note domain.Note, fileContent string) {
	t.Helper()

	structure, err := h.structureRepo.Load()
	if err != nil {
		t.Fatalf("plantNote load: %v", err)
	}

	filePath := filepath.Join(h.notesPath, note.NameOnDisk)
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		t.Fatalf("plantNote write: %v", err)
	}

	note.CreatedAt = time.Now()
	note.UpdatedAt = time.Now()
	structure.Notes = append(structure.Notes, note)

	if err := h.structureRepo.Save(structure); err != nil {
		t.Fatalf("plantNote save: %v", err)
	}
}
