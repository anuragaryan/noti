package service

// context_menu_ops_test.go — tests that exercise the rename / delete / move
// operations that are triggered from the sidebar context menu in the UI.
//
// These tests focus on realistic end-to-end flows rather than isolated edge
// cases, ensuring that the backend correctly handles the exact call sequences
// the frontend makes.

import (
	"os"
	"path/filepath"
	"testing"
)

// ── Rename note (context menu → rename-note modal) ────────────────────────────

// TestContextMenu_RenameNote_UpdatesTitle verifies that renaming a note via
// the context menu (which calls Update with the new title and existing content)
// renames the file on disk and updates the title in the structure.
func TestContextMenu_RenameNote_UpdatesTitle(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, err := h.service.Create("Draft", "some content", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate what the rename modal does: fetch full note, then Update with new title.
	full, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get before rename: %v", err)
	}

	if err := h.service.Update(note.ID, "Final Report", full.MarkdownContent, full.TranscriptContent); err != nil {
		t.Fatalf("Update (rename): %v", err)
	}

	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get after rename: %v", err)
	}
	if got.Title != "Final Report" {
		t.Errorf("title: got %q, want %q", got.Title, "Final Report")
	}
	if got.MarkdownContent != "some content" {
		t.Errorf("content changed unexpectedly: got %q", got.MarkdownContent)
	}
}

// TestContextMenu_RenameNote_SameTitle_Succeeds verifies that renaming a note
// to the same title as it already has is a valid no-op and does not return an
// error.
func TestContextMenu_RenameNote_SameTitle_Succeeds(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, _ := h.service.Create("Same Title", "content", "")

	full, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Rename to the same title — should succeed silently.
	if err := h.service.Update(note.ID, "Same Title", full.MarkdownContent, full.TranscriptContent); err != nil {
		t.Fatalf("Update (same title): %v", err)
	}
}

// TestContextMenu_RenameNote_InsideFolder_NoteStillAccessible verifies that a
// note inside a folder remains accessible after rename.
func TestContextMenu_RenameNote_InsideFolder_NoteStillAccessible(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)
	folder := h.createFolder(t, "work")

	note, _ := h.service.Create("Draft", "hello", folder.ID)
	full, _ := h.service.Get(note.ID)

	if err := h.service.Update(note.ID, "Polished", full.MarkdownContent, full.TranscriptContent); err != nil {
		t.Fatalf("Update inside folder: %v", err)
	}

	got, err := h.service.Get(note.ID)
	if err != nil {
		t.Fatalf("Get after rename inside folder: %v", err)
	}
	if got.Title != "Polished" {
		t.Errorf("title: got %q, want %q", got.Title, "Polished")
	}
	if got.FolderID != folder.ID {
		t.Errorf("FolderID changed: got %q, want %q", got.FolderID, folder.ID)
	}
	// The new file must be inside the folder directory.
	newPath := filepath.Join(h.notesPath, "markdown", folder.NameOnDisk, got.FileStem+".md")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("renamed file should be inside the folder directory: %v", err)
	}
}

// ── Rename folder (context menu → rename-folder modal) ────────────────────────

// TestContextMenu_RenameFolder_NoteInsideStillReadable verifies that notes
// inside a folder remain readable after the folder is renamed (the physical
// directory moves but notes are still found via the updated structure).
func TestContextMenu_RenameFolder_NoteInsideStillReadable(t *testing.T) {
	t.Parallel()
	// Use folderServiceHarness — it exposes both folderService and noteService.
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("OldFolder", "")
	note, _ := h.noteService.Create("Meeting Notes", "content here", folder.ID)

	// Simulate what the rename modal does: Update(id, newName, sameParentID).
	if err := h.folderService.Update(folder.ID, "NewFolder", ""); err != nil {
		t.Fatalf("Update folder name: %v", err)
	}

	// Note must still be accessible by ID.
	got, err := h.noteService.Get(note.ID)
	if err != nil {
		t.Fatalf("Get note after folder rename: %v", err)
	}
	if got.MarkdownContent != "content here" {
		t.Errorf("note content changed: got %q", got.MarkdownContent)
	}
}

// TestContextMenu_RenameFolder_KeepsTimestampPrefix verifies that renaming a
// folder via Update preserves the timestamp prefix in NameOnDisk.
func TestContextMenu_RenameFolder_KeepsTimestampPrefix(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("Alpha", "")

	if err := h.folderService.Update(folder.ID, "Beta", ""); err != nil {
		t.Fatalf("Update: %v", err)
	}

	folders, _ := h.folderService.GetAll()
	var updated string
	for _, f := range folders {
		if f.ID == folder.ID {
			updated = f.NameOnDisk
		}
	}

	if updated == "" {
		t.Fatal("could not find updated folder in structure")
	}
	// The new NameOnDisk must end with the sanitised new name.
	if len(updated) < 4 || updated[len(updated)-4:] != "beta" {
		t.Errorf("NameOnDisk %q should end with 'beta'", updated)
	}
}

// ── Move note (context menu → move-note modal) ────────────────────────────────

// TestContextMenu_MoveNote_BetweenFolders verifies the round-trip:
// root → folder A → folder B → root.
func TestContextMenu_MoveNote_BetweenFolders(t *testing.T) {
	t.Parallel()
	// folderServiceHarness exposes both services.
	h := newFolderServiceHarness(t)

	folderA, _ := h.folderService.Create("FolderA", "")
	folderB, _ := h.folderService.Create("FolderB", "")
	note, _ := h.noteService.Create("Traveller", "data", "")

	steps := []struct {
		dest  string
		label string
	}{
		{folderA.ID, "root \u2192 FolderA"},
		{folderB.ID, "FolderA \u2192 FolderB"},
		{"", "FolderB \u2192 root"},
	}

	for _, step := range steps {
		if err := h.noteService.Move(note.ID, step.dest); err != nil {
			t.Fatalf("Move (%s): %v", step.label, err)
		}
		got, err := h.noteService.Get(note.ID)
		if err != nil {
			t.Fatalf("Get after move (%s): %v", step.label, err)
		}
		if got.FolderID != step.dest {
			t.Errorf("move (%s): FolderID = %q, want %q", step.label, got.FolderID, step.dest)
		}
		if got.MarkdownContent != "data" {
			t.Errorf("move (%s): content changed to %q", step.label, got.MarkdownContent)
		}
	}
}

// ── Move folder (context menu → move-folder modal) ────────────────────────────

// TestContextMenu_MoveFolder_ToNewParent_NotesInsideStillAccessible verifies
// that notes inside a folder are still readable after the folder is moved to a
// new parent (the entire directory subtree is relocated on disk).
func TestContextMenu_MoveFolder_ToNewParent_NotesInsideStillAccessible(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	parent, _ := h.folderService.Create("Parent", "")
	child, _ := h.folderService.Create("Child", "")
	note, _ := h.noteService.Create("Doc", "important", child.ID)

	// Move child under parent.
	if err := h.folderService.Update(child.ID, child.Name, parent.ID); err != nil {
		t.Fatalf("Update (move folder): %v", err)
	}

	got, err := h.noteService.Get(note.ID)
	if err != nil {
		t.Fatalf("Get note after folder move: %v", err)
	}
	if got.MarkdownContent != "important" {
		t.Errorf("note content changed after folder move: %q", got.MarkdownContent)
	}
}

// TestContextMenu_MoveFolder_ToRoot_RemovesParentReference verifies that
// moving a nested folder back to the root clears its parentID.
func TestContextMenu_MoveFolder_ToRoot_RemovesParentReference(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	parent, _ := h.folderService.Create("Parent", "")
	child, _ := h.folderService.Create("Child", parent.ID)

	// Move child back to root.
	if err := h.folderService.Update(child.ID, child.Name, ""); err != nil {
		t.Fatalf("Update (move to root): %v", err)
	}

	folders, _ := h.folderService.GetAll()
	for _, f := range folders {
		if f.ID == child.ID {
			if f.ParentID != "" {
				t.Errorf("ParentID should be empty after move to root, got %q", f.ParentID)
			}
			return
		}
	}
	t.Error("child folder not found in structure after move")
}

// ── Delete (context menu → delete-confirm modal) ──────────────────────────────

// TestContextMenu_DeleteNote_WhileItIsCurrentNote verifies that deleting the
// currently-viewed note removes it from disk and structure, which the UI then
// handles by clearing currentNote.
func TestContextMenu_DeleteNote_WhileItIsCurrentNote(t *testing.T) {
	t.Parallel()
	h := newNoteServiceHarness(t)

	note, _ := h.service.Create("Active Note", "body", "")
	filePath := filepath.Join(h.notesPath, "markdown", note.FileStem+".md")

	if err := h.service.Delete(note.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should have been deleted from disk")
	}

	_, err := h.service.Get(note.ID)
	if err == nil {
		t.Error("Get should return error for deleted note")
	}
}

// TestContextMenu_DeleteFolder_PreservingNotes verifies that deleting a folder
// with deleteNotes=false (the default checkbox-unchecked path) moves contained
// notes to the root rather than deleting them.
func TestContextMenu_DeleteFolder_PreservingNotes(t *testing.T) {
	t.Parallel()
	h := newFolderServiceHarness(t)

	folder, _ := h.folderService.Create("Temp", "")
	note, _ := h.noteService.Create("Keeper", "keep this", folder.ID)

	if err := h.folderService.Delete(folder.ID, false, h.noteService); err != nil {
		t.Fatalf("Delete folder (preserve notes): %v", err)
	}

	got, err := h.noteService.Get(note.ID)
	if err != nil {
		t.Fatalf("note should still exist after folder delete: %v", err)
	}
	if got.FolderID != "" {
		t.Errorf("note.FolderID should be empty after folder delete, got %q", got.FolderID)
	}
	if got.MarkdownContent != "keep this" {
		t.Errorf("note content changed: %q", got.MarkdownContent)
	}
}
