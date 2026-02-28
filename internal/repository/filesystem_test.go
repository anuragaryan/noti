package repository

import (
	"os"
	"path/filepath"
	"testing"

	"noti/internal/domain"
)

// setupFilesystemTest creates a temp directory, a PathResolver, and a
// FileSystem ready for use. The caller receives the notes root path.
func setupFilesystemTest(t *testing.T) (fs *FileSystem, notesPath string) {
	t.Helper()
	notesPath = t.TempDir()
	pr := NewPathResolver(notesPath)
	fs = NewFileSystem(pr)
	return fs, notesPath
}

// singleNoteStructure returns a structure with one root-level note already
// written to disk so tests can call Load/Save/Move/Delete without extra setup.
func singleNoteStructure(t *testing.T, notesPath, noteID, fileName, content string) *domain.FolderStructure {
	t.Helper()
	path := filepath.Join(notesPath, fileName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("setup: could not write note file: %v", err)
	}
	return &domain.FolderStructure{
		Notes: []domain.Note{
			{ID: noteID, NameOnDisk: fileName, FolderID: ""},
		},
	}
}

// ── LoadNoteContent ───────────────────────────────────────────────────────────

func TestLoadNoteContent_ReadsFileFromDisk(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)
	structure := singleNoteStructure(t, notesPath, "n1", "hello.md", "hello world")

	content, err := fs.LoadNoteContent(&structure.Notes[0], structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "hello world" {
		t.Errorf("got %q, want %q", content, "hello world")
	}
}

func TestLoadNoteContent_MissingFile_ReturnsError(t *testing.T) {
	t.Parallel()
	fs, _ := setupFilesystemTest(t)
	structure := &domain.FolderStructure{
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "ghost.md", FolderID: ""},
		},
	}

	_, err := fs.LoadNoteContent(&structure.Notes[0], structure)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ── SaveNoteContent ───────────────────────────────────────────────────────────

func TestSaveNoteContent_WritesContentToDisk(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)
	structure := singleNoteStructure(t, notesPath, "n1", "note.md", "")

	if err := fs.SaveNoteContent(&structure.Notes[0], "updated content", structure); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(notesPath, "note.md"))
	if string(data) != "updated content" {
		t.Errorf("got %q, want %q", string(data), "updated content")
	}
}

// TestSaveNoteContent_CreatesParentDirectoryWhenMissing verifies that
// SaveNoteContent creates the parent directory if it does not yet exist.
func TestSaveNoteContent_CreatesParentDirectoryWhenMissing(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)

	// Register a folder in the structure but do NOT create the directory on
	// disk — SaveNoteContent must create it via MkdirAll.
	structure := &domain.FolderStructure{
		Folders: []domain.Folder{
			{ID: "f1", NameOnDisk: "newdir", ParentID: ""},
		},
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "nested.md", FolderID: "f1"},
		},
	}

	if err := fs.SaveNoteContent(&structure.Notes[0], "nested content", structure); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(notesPath, "newdir", "nested.md"))
	if err != nil {
		t.Fatalf("file should have been created: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("got %q, want %q", string(data), "nested content")
	}
}

// ── MoveNoteFile ──────────────────────────────────────────────────────────────

func TestMoveNoteFile_MovesFileToNewLocation(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)

	// Create source file at root.
	srcPath := filepath.Join(notesPath, "move-me.md")
	if err := os.WriteFile(srcPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create destination folder on disk.
	destFolder := filepath.Join(notesPath, "dest")
	if err := os.MkdirAll(destFolder, 0755); err != nil {
		t.Fatal(err)
	}

	structure := &domain.FolderStructure{
		Folders: []domain.Folder{
			{ID: "f1", NameOnDisk: "dest", ParentID: ""},
		},
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "move-me.md", FolderID: ""},
		},
	}

	note := &structure.Notes[0]
	if err := fs.MoveNoteFile(note, "", "f1", structure, notesPath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Error("source file should have been removed")
	}
	destPath := filepath.Join(destFolder, "move-me.md")
	if _, err := os.Stat(destPath); err != nil {
		t.Errorf("destination file should exist: %v", err)
	}
}

// TestMoveNoteFile_SameSource_IsNoOp verifies that moving a note to its current
// folder is a no-op and leaves the file in place.
func TestMoveNoteFile_SameSource_IsNoOp(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)

	filePath := filepath.Join(notesPath, "same.md")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	structure := &domain.FolderStructure{
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "same.md", FolderID: ""},
		},
	}

	if err := fs.MoveNoteFile(&structure.Notes[0], "", "", structure, notesPath); err != nil {
		t.Fatalf("unexpected error on no-op move: %v", err)
	}

	// Verify the file is still present after the no-op.
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("file should still exist after no-op move: %v", err)
	}
}

// TestMoveNoteFile_UnknownNewFolder_ReturnsError verifies that passing an
// unresolvable destination folder ID returns an error.
func TestMoveNoteFile_UnknownNewFolder_ReturnsError(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)

	if err := os.WriteFile(filepath.Join(notesPath, "note.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	structure := &domain.FolderStructure{
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "note.md", FolderID: ""},
		},
		// No folders registered — "nonexistent" cannot be resolved.
	}

	err := fs.MoveNoteFile(&structure.Notes[0], "", "nonexistent", structure, notesPath)
	if err == nil {
		t.Fatal("expected error for unresolvable new folder, got nil")
	}
}

// ── DeleteNoteFile ────────────────────────────────────────────────────────────

func TestDeleteNoteFile_RemovesFileFromDisk(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)
	structure := singleNoteStructure(t, notesPath, "n1", "delete-me.md", "bye")

	if err := fs.DeleteNoteFile(&structure.Notes[0], structure); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(notesPath, "delete-me.md")); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestDeleteNoteFile_MissingFile_ReturnsError(t *testing.T) {
	t.Parallel()
	fs, _ := setupFilesystemTest(t)
	structure := &domain.FolderStructure{
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "gone.md", FolderID: ""},
		},
	}

	err := fs.DeleteNoteFile(&structure.Notes[0], structure)
	if err == nil {
		t.Fatal("expected error when deleting non-existent file, got nil")
	}
}

// TestDeleteNoteFile_UnresolvablePath_ReturnsError verifies that an error is
// returned when the note's FolderID refers to a folder absent from the structure.
func TestDeleteNoteFile_UnresolvablePath_ReturnsError(t *testing.T) {
	t.Parallel()
	fs, _ := setupFilesystemTest(t)

	structure := &domain.FolderStructure{
		// "missing-folder" is not in Folders — GetPathFor will error.
		Notes: []domain.Note{
			{ID: "n1", NameOnDisk: "note.md", FolderID: "missing-folder"},
		},
	}

	err := fs.DeleteNoteFile(&structure.Notes[0], structure)
	if err == nil {
		t.Fatal("expected error when note's folder cannot be resolved, got nil")
	}
}
