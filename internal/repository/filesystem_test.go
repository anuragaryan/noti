package repository

import (
	"os"
	"path/filepath"
	"testing"

	"noti/internal/domain"
)

func setupFilesystemTest(t *testing.T) (fs *FileSystem, notesPath string) {
	t.Helper()
	notesPath = t.TempDir()
	pr := NewPathResolver(notesPath)
	fs = NewFileSystem(pr)
	return fs, notesPath
}

func singleNoteStructure(fileStem string) *domain.FolderStructure {
	return &domain.FolderStructure{
		Notes: []domain.Note{{ID: "n1", FileStem: fileStem, FolderID: ""}},
	}
}

func TestLoadNoteContentPair_ReadsBothFilesFromDisk(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)
	structure := singleNoteStructure("hello")

	if err := os.MkdirAll(filepath.Join(notesPath, "markdown"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(notesPath, "transcripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesPath, "markdown", "hello.md"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesPath, "transcripts", "hello.transcript.txt"), []byte("raw words"), 0o644); err != nil {
		t.Fatal(err)
	}

	markdown, transcript, err := fs.LoadNoteContentPair(&structure.Notes[0], structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if markdown != "hello world" {
		t.Errorf("markdown: got %q, want %q", markdown, "hello world")
	}
	if transcript != "raw words" {
		t.Errorf("transcript: got %q, want %q", transcript, "raw words")
	}
}

func TestLoadNoteContentPair_MissingFiles_ReturnsEmptyStrings(t *testing.T) {
	t.Parallel()
	fs, _ := setupFilesystemTest(t)
	structure := singleNoteStructure("ghost")

	markdown, transcript, err := fs.LoadNoteContentPair(&structure.Notes[0], structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if markdown != "" || transcript != "" {
		t.Fatalf("expected both contents empty, got markdown=%q transcript=%q", markdown, transcript)
	}
}

func TestSaveNoteContentPair_WritesBothFiles(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)
	structure := singleNoteStructure("note")

	if err := fs.SaveNoteContentPair(&structure.Notes[0], "updated content", "captured transcript", structure); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	markdown, err := os.ReadFile(filepath.Join(notesPath, "markdown", "note.md"))
	if err != nil {
		t.Fatalf("markdown file should exist: %v", err)
	}
	transcript, err := os.ReadFile(filepath.Join(notesPath, "transcripts", "note.transcript.txt"))
	if err != nil {
		t.Fatalf("transcript file should exist: %v", err)
	}

	if string(markdown) != "updated content" {
		t.Errorf("markdown: got %q, want %q", string(markdown), "updated content")
	}
	if string(transcript) != "captured transcript" {
		t.Errorf("transcript: got %q, want %q", string(transcript), "captured transcript")
	}
}

func TestMoveNoteFilePair_MovesBothFilesToNewFolder(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)

	structure := &domain.FolderStructure{
		Folders: []domain.Folder{{ID: "f1", NameOnDisk: "dest", ParentID: ""}},
		Notes:   []domain.Note{{ID: "n1", FileStem: "move-me", FolderID: ""}},
	}

	if err := os.MkdirAll(filepath.Join(notesPath, "markdown"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(notesPath, "transcripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesPath, "markdown", "move-me.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesPath, "transcripts", "move-me.transcript.txt"), []byte("raw"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := fs.MoveNoteFilePair(&structure.Notes[0], "", "f1", structure); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(notesPath, "markdown", "move-me.md")); !os.IsNotExist(err) {
		t.Error("source markdown file should have been removed")
	}
	if _, err := os.Stat(filepath.Join(notesPath, "transcripts", "move-me.transcript.txt")); !os.IsNotExist(err) {
		t.Error("source transcript file should have been removed")
	}
	if _, err := os.Stat(filepath.Join(notesPath, "markdown", "dest", "move-me.md")); err != nil {
		t.Errorf("destination markdown file should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(notesPath, "transcripts", "dest", "move-me.transcript.txt")); err != nil {
		t.Errorf("destination transcript file should exist: %v", err)
	}
}

func TestDeleteNoteFilePair_RemovesBothFiles(t *testing.T) {
	t.Parallel()
	fs, notesPath := setupFilesystemTest(t)
	structure := singleNoteStructure("delete-me")

	if err := os.MkdirAll(filepath.Join(notesPath, "markdown"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(notesPath, "transcripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesPath, "markdown", "delete-me.md"), []byte("bye"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesPath, "transcripts", "delete-me.transcript.txt"), []byte("raw"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := fs.DeleteNoteFilePair(&structure.Notes[0], structure); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(notesPath, "markdown", "delete-me.md")); !os.IsNotExist(err) {
		t.Error("markdown file should have been deleted")
	}
	if _, err := os.Stat(filepath.Join(notesPath, "transcripts", "delete-me.transcript.txt")); !os.IsNotExist(err) {
		t.Error("transcript file should have been deleted")
	}
}
