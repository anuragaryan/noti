package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptService_loadPromptFromFile_MissingID_ReturnsError(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	svc := NewPromptService(basePath)

	content := "---\n{\"name\":\"Summarize\"}\n---\n\n## System Prompt\n\nSystem\n\n## User Prompt\n\nUser\n"
	path := filepath.Join(basePath, "prompts", "broken.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create prompt dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	_, err := svc.loadPromptFromFile(path)
	if err == nil {
		t.Fatal("expected error for missing id, got nil")
	}
	if !strings.Contains(err.Error(), "missing or non-string id") {
		t.Fatalf("expected id validation error, got: %v", err)
	}
}

func TestPromptService_loadPromptFromFile_NonStringName_ReturnsError(t *testing.T) {
	t.Parallel()

	basePath := t.TempDir()
	svc := NewPromptService(basePath)

	content := "---\n{\"id\":\"p1\",\"name\":123}\n---\n\n## System Prompt\n\nSystem\n\n## User Prompt\n\nUser\n"
	path := filepath.Join(basePath, "prompts", "broken-name.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create prompt dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	_, err := svc.loadPromptFromFile(path)
	if err == nil {
		t.Fatal("expected error for non-string name, got nil")
	}
	if !strings.Contains(err.Error(), "missing or non-string name") {
		t.Fatalf("expected name validation error, got: %v", err)
	}
}
