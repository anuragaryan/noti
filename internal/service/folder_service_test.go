package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"noti/internal/domain"
	"noti/internal/repository"
)

// folderServiceHarness wires up both FolderService and NoteService backed by a
// real temp directory. Many folder operations need a NoteService reference.
type folderServiceHarness struct {
	notesPath     string
	structureRepo *repository.StructureRepository
	folderService *FolderService
	noteService   *NoteService
}

func newFolderServiceHarness(t *testing.T) *folderServiceHarness {
	t.Helper()
	notesPath := t.TempDir()

	structurePath := filepath.Join(notesPath, "structure.json")
	structureRepo := repository.NewStructureRepository(structurePath)
	pathResolver := repository.NewPathResolver(notesPath)
	fs := repository.NewFileSystem(pathResolver)

	folderSvc := NewFolderService(structureRepo, pathResolver, notesPath)
	noteSvc := NewNoteService(structureRepo, pathResolver, fs, notesPath)

	if err := structureRepo.Save(&domain.FolderStructure{
		Folders: []domain.Folder{},
		Notes:   []domain.Note{},
	}); err != nil {
		t.Fatalf("harness: could not seed structure: %v", err)
	}

	return &folderServiceHarness{
		notesPath:     notesPath,
		structureRepo: structureRepo,
		folderService: folderSvc,
		noteService:   noteSvc,
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestFolderService_Create_CreatesFolderOnDiskAndInStructure(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, err := h.folderService.Create("Work", "")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	if folder.ID == "" {
		t.Error("folder.ID should not be empty")
	}

	// Directory must exist on disk.
	dirPath := filepath.Join(h.notesPath, "markdown", folder.NameOnDisk)
	if info, err := os.Stat(dirPath); err != nil || !info.IsDir() {
		t.Errorf("folder directory should exist on disk at %q", dirPath)
	}
}

func TestFolderService_Create_UnknownParent_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	_, err := h.folderService.Create("Child", "nonexistent-parent")
	if err == nil {
		t.Fatal("expected error for unknown parent folder, got nil")
	}
}

// TestFolderService_Create_OrderNeverDuplicatesAfterDeletion verifies that no
// two active folders share the same Order value, even after one is deleted.
func TestFolderService_Create_OrderNeverDuplicatesAfterDeletion(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	f1, err := h.folderService.Create("A", "")
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	f2, err := h.folderService.Create("B", "")
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	if err := h.folderService.Delete(f2.ID, false, h.noteService); err != nil {
		t.Fatalf("Delete B: %v", err)
	}
	f3, err := h.folderService.Create("C", "")
	if err != nil {
		t.Fatalf("Create C: %v", err)
	}
	// No two active folders may share the same Order; f3 must not collide with f1.
	if f3.Order == f1.Order {
		t.Errorf("f3.Order (%d) must not equal f1.Order (%d): active folders have duplicate orders", f3.Order, f1.Order)
	}
}

func TestFolderService_Create_NestedFolder_PlacedInsideParentDirectory(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	parent, _ := h.folderService.Create("Parent", "")
	child, err := h.folderService.Create("Child", parent.ID)
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}

	childPath := filepath.Join(h.notesPath, "markdown", parent.NameOnDisk, child.NameOnDisk)
	if info, err := os.Stat(childPath); err != nil || !info.IsDir() {
		t.Errorf("child directory should exist inside parent at %q", childPath)
	}
}

// ── GetAll ────────────────────────────────────────────────────────────────────

func TestFolderService_GetAll_ReturnsAllFolders(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	for _, name := range []string{"A", "B", "C"} {
		if _, err := h.folderService.Create(name, ""); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}

	folders, err := h.folderService.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(folders) != 3 {
		t.Errorf("expected 3 folders, got %d", len(folders))
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

// TestFolderService_Update_RenamesMovesFolderOnDisk verifies that the timestamp
// prefix in NameOnDisk is preserved after a rename.
func TestFolderService_Update_RenamesMovesFolderOnDisk(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("OldName", "")
	oldPath := filepath.Join(h.notesPath, "markdown", folder.NameOnDisk)

	if err := h.folderService.Update(folder.ID, "NewName", ""); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Old directory must be gone.
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old folder directory should have been renamed")
	}

	// Verify the structure recorded the exact new NameOnDisk, not just the Name.
	folders, err := h.folderService.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	var updated domain.Folder
	for _, f := range folders {
		if f.ID == folder.ID {
			updated = f
		}
	}
	if updated.Name != "NewName" {
		t.Errorf("folder Name in structure: got %q, want %q", updated.Name, "NewName")
	}
	if !strings.HasSuffix(updated.NameOnDisk, "newname") {
		t.Errorf("NameOnDisk %q should end with sanitised new name %q", updated.NameOnDisk, "newname")
	}
	newPath := filepath.Join(h.notesPath, "markdown", updated.NameOnDisk)
	if info, err := os.Stat(newPath); err != nil || !info.IsDir() {
		t.Errorf("renamed folder directory should exist at %q", newPath)
	}
}

func TestFolderService_Update_MoveToNewParent_MovesDirectoryOnDisk(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	parent, _ := h.folderService.Create("Parent", "")
	child, _ := h.folderService.Create("Child", "")

	oldChildPath := filepath.Join(h.notesPath, "markdown", child.NameOnDisk)

	if err := h.folderService.Update(child.ID, "Child", parent.ID); err != nil {
		t.Fatalf("Update (move): %v", err)
	}

	if _, err := os.Stat(oldChildPath); !os.IsNotExist(err) {
		t.Error("child directory should have moved out of root")
	}

	newChildPath := filepath.Join(h.notesPath, "markdown", parent.NameOnDisk, child.NameOnDisk)
	if info, err := os.Stat(newChildPath); err != nil || !info.IsDir() {
		t.Errorf("child directory should now be inside parent at %q", newChildPath)
	}
}

func TestFolderService_Update_CircularMove_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	parent, _ := h.folderService.Create("Parent", "")
	child, _ := h.folderService.Create("Child", parent.ID)

	// Try to make Parent a child of Child — would create a cycle.
	err := h.folderService.Update(parent.ID, "Parent", child.ID)
	if err == nil {
		t.Fatal("expected error for circular move, got nil")
	}
}

// TestFolderService_Update_ThreeLevelCircularMove_ReturnsError verifies that
// a cycle spanning three levels (A→B→C→A) is correctly rejected.
func TestFolderService_Update_ThreeLevelCircularMove_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	a, _ := h.folderService.Create("A", "")
	b, _ := h.folderService.Create("B", a.ID)
	c, _ := h.folderService.Create("C", b.ID)

	// Moving A under C would create A→B→C→A cycle.
	err := h.folderService.Update(a.ID, "A", c.ID)
	if err == nil {
		t.Fatal("expected circular reference error for 3-level cycle, got nil")
	}
}

func TestFolderService_Update_MoveIntoSelf_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	f, _ := h.folderService.Create("Folder", "")
	err := h.folderService.Update(f.ID, "Folder", f.ID)
	if err == nil {
		t.Fatal("expected error when moving folder into itself, got nil")
	}
}

func TestFolderService_Update_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	err := h.folderService.Update("ghost", "Name", "")
	if err == nil {
		t.Fatal("expected error for unknown folder ID, got nil")
	}
}

func TestFolderService_Update_NameOnDiskWithNoDash_DoesNotPanic(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	// Manually plant a folder whose NameOnDisk has no dash.
	svc := h.folderService
	svc.mu.Lock()
	structure, _ := svc.structureRepo.Load()
	dirPath := filepath.Join(h.notesPath, "markdown", "nodash")
	_ = os.MkdirAll(dirPath, 0755)
	transcriptDirPath := filepath.Join(h.notesPath, "transcripts", "nodash")
	_ = os.MkdirAll(transcriptDirPath, 0755)
	structure.Folders = append(structure.Folders, domain.Folder{
		ID:         "fd1",
		Name:       "OldName",
		NameOnDisk: "nodash", // no dash — exercises renamedDiskName edge case
		ParentID:   "",
	})
	if err := svc.structureRepo.Save(structure); err != nil {
		svc.mu.Unlock()
		t.Fatalf("setup save: %v", err)
	}
	svc.mu.Unlock()

	if err := h.folderService.Update("fd1", "NewName", ""); err != nil {
		t.Fatalf("Update with no-dash NameOnDisk: %v", err)
	}
}

// ── Delete (deleteNotes = true) ───────────────────────────────────────────────

// TestFolderService_Delete_WithDeleteNotes_RemovesFolderAndNotes verifies that
// both the folder directory and every note file inside it are removed from disk.
func TestFolderService_Delete_WithDeleteNotes_RemovesFolderAndNotes(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("Inbox", "")
	note, _ := h.noteService.Create("Note inside", "content", folder.ID)

	// Capture the expected file path before deletion.
	noteFilePath := filepath.Join(h.notesPath, "markdown", folder.NameOnDisk, note.FileStem+".md")
	folderPath := filepath.Join(h.notesPath, "markdown", folder.NameOnDisk)

	if err := h.folderService.Delete(folder.ID, true, h.noteService); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Folder directory must be gone.
	if _, err := os.Stat(folderPath); !os.IsNotExist(err) {
		t.Error("folder directory should have been removed")
	}

	// Note file must be physically deleted from disk, not just removed from structure.
	if _, err := os.Stat(noteFilePath); !os.IsNotExist(err) {
		t.Error("note file should have been deleted from disk")
	}

	// Note must not appear in GetAll.
	notes, _ := h.noteService.GetAll()
	for _, n := range notes {
		if n.ID == note.ID {
			t.Error("deleted note should not appear in GetAll after folder delete")
		}
	}
}

// TestFolderService_Delete_WithDeleteNotes_EmptyFolder_Succeeds verifies that
// deleting an empty folder with deleteNotes=true completes without error.
func TestFolderService_Delete_WithDeleteNotes_EmptyFolder_Succeeds(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("Empty", "")
	folderPath := filepath.Join(h.notesPath, "markdown", folder.NameOnDisk)

	if err := h.folderService.Delete(folder.ID, true, h.noteService); err != nil {
		t.Fatalf("Delete empty folder with deleteNotes=true: %v", err)
	}

	if _, err := os.Stat(folderPath); !os.IsNotExist(err) {
		t.Error("empty folder directory should have been removed")
	}

	folders, _ := h.folderService.GetAll()
	for _, f := range folders {
		if f.ID == folder.ID {
			t.Error("deleted empty folder should not appear in GetAll")
		}
	}
}

// ── Delete (deleteNotes = false) ──────────────────────────────────────────────

func TestFolderService_Delete_WithoutDeleteNotes_MovesNotesToRoot(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("Temp", "")
	note, _ := h.noteService.Create("Orphan", "content", folder.ID)

	if err := h.folderService.Delete(folder.ID, false, h.noteService); err != nil {
		t.Fatalf("Delete (move notes): %v", err)
	}

	// Note should now be at root in the structure.
	got, err := h.noteService.Get(note.ID)
	if err != nil {
		t.Fatalf("Get note after folder delete: %v", err)
	}
	if got.FolderID != "" {
		t.Errorf("note.FolderID should be empty after folder delete, got %q", got.FolderID)
	}

	// File must physically be at the notes root.
	rootFilePath := filepath.Join(h.notesPath, "markdown", note.FileStem+".md")
	if _, err := os.Stat(rootFilePath); err != nil {
		t.Errorf("note file should be at root after folder delete: %v", err)
	}
}

func TestFolderService_Delete_WithSubfolders_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	parent, _ := h.folderService.Create("Parent", "")
	h.folderService.Create("Child", parent.ID)

	err := h.folderService.Delete(parent.ID, false, h.noteService)
	if err == nil {
		t.Fatal("expected error when deleting folder with subfolders, got nil")
	}
}

func TestFolderService_Delete_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	err := h.folderService.Delete("ghost", false, h.noteService)
	if err == nil {
		t.Fatal("expected error for unknown folder ID, got nil")
	}
}

// ── GetPath ───────────────────────────────────────────────────────────────────

func TestFolderService_GetPath_EmptyID_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	path, err := h.folderService.GetPath("")
	if err != nil {
		t.Fatalf("GetPath: %v", err)
	}
	if len(path) != 0 {
		t.Errorf("expected empty path for empty ID, got %v", path)
	}
}

func TestFolderService_GetPath_NestedFolder_ReturnsBreadcrumb(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	root, _ := h.folderService.Create("Root", "")
	mid, _ := h.folderService.Create("Mid", root.ID)
	leaf, _ := h.folderService.Create("Leaf", mid.ID)

	path, err := h.folderService.GetPath(leaf.ID)
	if err != nil {
		t.Fatalf("GetPath: %v", err)
	}

	if len(path) != 3 {
		t.Fatalf("expected 3 breadcrumbs, got %d: %v", len(path), path)
	}
	if path[0].ID != root.ID || path[1].ID != mid.ID || path[2].ID != leaf.ID {
		t.Errorf("breadcrumb order wrong: got %v", path)
	}
}

func TestFolderService_GetPath_RootFolder_ReturnsSingleEntry(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("Solo", "")
	path, err := h.folderService.GetPath(folder.ID)
	if err != nil {
		t.Fatalf("GetPath: %v", err)
	}
	if len(path) != 1 || path[0].ID != folder.ID {
		t.Errorf("expected single-entry path, got %v", path)
	}
}

// TestFolderService_GetPath_UnknownFolderID_ReturnsEmptySlice documents that
// GetPath returns an empty slice (no error) when the folderID is not found.
func TestFolderService_GetPath_UnknownFolderID_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	path, err := h.folderService.GetPath("does-not-exist")
	if err != nil {
		t.Fatalf("GetPath with unknown ID: expected no error, got: %v", err)
	}
	// The production code breaks out of the walk loop when the ID is not found
	// and returns whatever it has accumulated so far — which is nothing.
	if len(path) != 0 {
		t.Errorf("expected empty path for unknown folder ID, got %v", path)
	}
}
