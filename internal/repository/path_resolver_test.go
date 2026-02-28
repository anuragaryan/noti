package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"noti/internal/domain"
)

// makeStructure is a helper that builds a FolderStructure from a compact
// description so tests stay readable.
func makeStructure(folders []domain.Folder, notes []domain.Note) *domain.FolderStructure {
	return &domain.FolderStructure{Folders: folders, Notes: notes}
}

func TestGetPathFor_RootNote(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(nil, []domain.Note{
		{ID: "n1", NameOnDisk: "note.md", FolderID: ""},
	})

	got, err := r.GetPathFor("n1", structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/notes", "note.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetPathFor_NoteInsideFolder(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(
		[]domain.Folder{
			{ID: "f1", NameOnDisk: "work", ParentID: ""},
		},
		[]domain.Note{
			{ID: "n1", NameOnDisk: "todo.md", FolderID: "f1"},
		},
	)

	got, err := r.GetPathFor("n1", structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/notes", "work", "todo.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetPathFor_NoteInsideNestedFolders(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(
		[]domain.Folder{
			{ID: "f1", NameOnDisk: "work", ParentID: ""},
			{ID: "f2", NameOnDisk: "q1", ParentID: "f1"},
		},
		[]domain.Note{
			{ID: "n1", NameOnDisk: "report.md", FolderID: "f2"},
		},
	)

	got, err := r.GetPathFor("n1", structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/notes", "work", "q1", "report.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetPathFor_RootFolder(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(
		[]domain.Folder{
			{ID: "f1", NameOnDisk: "personal", ParentID: ""},
		},
		nil,
	)

	got, err := r.GetPathFor("f1", structure)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/notes", "personal")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetPathFor_UnknownID_ReturnsError(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(nil, nil)

	_, err := r.GetPathFor("ghost", structure)
	if err == nil {
		t.Fatal("expected an error for unknown ID, got nil")
	}
}

func TestGetPathFor_CircularFolderReference_ReturnsError(t *testing.T) {
	t.Parallel()
	// f1 → f2 → f1 forms a cycle.
	r := NewPathResolver("/notes")
	structure := makeStructure(
		[]domain.Folder{
			{ID: "f1", NameOnDisk: "a", ParentID: "f2"},
			{ID: "f2", NameOnDisk: "b", ParentID: "f1"},
		},
		nil,
	)

	_, err := r.GetPathFor("f1", structure)
	if err == nil {
		t.Fatal("expected a cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected 'circular' in error message, got: %v", err)
	}
}

func TestGetPathFor_NoteFolderIDPointsToMissingFolder_ReturnsError(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(
		nil,
		[]domain.Note{
			{ID: "n1", NameOnDisk: "note.md", FolderID: "missing"},
		},
	)

	_, err := r.GetPathFor("n1", structure)
	if err == nil {
		t.Fatal("expected error for missing parent folder, got nil")
	}
}

func TestGetPathFor_SelfReferencingFolder_ReturnsError(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(
		[]domain.Folder{
			{ID: "f1", NameOnDisk: "loop", ParentID: "f1"}, // points to itself
		},
		nil,
	)

	_, err := r.GetPathFor("f1", structure)
	if err == nil {
		t.Fatal("expected cycle error for self-referencing folder, got nil")
	}
}

func TestGetPathFor_EmptyStructure_ReturnsError(t *testing.T) {
	t.Parallel()
	r := NewPathResolver("/notes")
	structure := makeStructure(nil, nil)

	_, err := r.GetPathFor("anything", structure)
	if err == nil {
		t.Fatal("expected error for empty structure, got nil")
	}
}
