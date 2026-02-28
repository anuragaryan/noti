package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"noti/internal/domain"
)

func newStructureRepo(t *testing.T) (*StructureRepository, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "structure.json")
	return NewStructureRepository(path), path
}

func TestStructureRepository_SaveAndLoad_RoundTrip(t *testing.T) {
	t.Parallel()
	repo, _ := newStructureRepo(t)

	original := &domain.FolderStructure{
		Folders: []domain.Folder{
			{ID: "f1", Name: "Work", NameOnDisk: "work", ParentID: "", Order: 0, CreatedAt: time.Now().Truncate(time.Second)},
		},
		Notes: []domain.Note{
			{ID: "n1", Title: "Todo", NameOnDisk: "todo.md", FolderID: "f1", Order: 0, CreatedAt: time.Now().Truncate(time.Second)},
		},
	}

	if err := repo.Save(original); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	loaded, err := repo.Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if len(loaded.Folders) != 1 || loaded.Folders[0].ID != "f1" {
		t.Errorf("folder round-trip failed: got %+v", loaded.Folders)
	}
	if len(loaded.Notes) != 1 || loaded.Notes[0].ID != "n1" {
		t.Errorf("note round-trip failed: got %+v", loaded.Notes)
	}
}

func TestStructureRepository_Load_MissingFile_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := NewStructureRepository("/tmp/does-not-exist/structure.json")

	_, err := repo.Load()
	if err == nil {
		t.Fatal("expected error when file does not exist, got nil")
	}
}

func TestStructureRepository_Load_CorruptJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	repo, path := newStructureRepo(t)

	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := repo.Load()
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}
}

func TestStructureRepository_Save_CreatesDirectoryIfMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Use a nested path that does not exist yet.
	path := filepath.Join(dir, "sub", "dir", "structure.json")
	repo := NewStructureRepository(path)

	if err := repo.Save(&domain.FolderStructure{}); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected structure.json to be created: %v", err)
	}
}

func TestStructureRepository_Save_WritesValidJSON(t *testing.T) {
	t.Parallel()
	repo, path := newStructureRepo(t)

	structure := &domain.FolderStructure{
		Folders: []domain.Folder{{ID: "f1", Name: "X"}},
		Notes:   []domain.Note{{ID: "n1", Title: "Y"}},
	}

	if err := repo.Save(structure); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var parsed domain.FolderStructure
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
}

func TestStructureRepository_SaveEmptyStructure_RoundTrips(t *testing.T) {
	t.Parallel()
	repo, _ := newStructureRepo(t)

	empty := &domain.FolderStructure{
		Folders: []domain.Folder{},
		Notes:   []domain.Note{},
	}

	if err := repo.Save(empty); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	loaded, err := repo.Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if loaded.Folders == nil || loaded.Notes == nil {
		t.Error("loaded structure should have non-nil slices for empty arrays")
	}
}
