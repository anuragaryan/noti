package util

import (
	"strings"
	"testing"
)

func TestSanitizeName_ReplacesSpacesWithHyphens(t *testing.T) {
	t.Parallel()
	got := SanitizeName("hello world")
	if got != "hello-world" {
		t.Errorf("got %q, want %q", got, "hello-world")
	}
}

// TestSanitizeName_LowercasesInput verifies the exact lowercased-and-hyphenated output.
func TestSanitizeName_LowercasesInput(t *testing.T) {
	t.Parallel()
	got := SanitizeName("Hello World")
	want := "hello-world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeName_ReplacesForwardSlash(t *testing.T) {
	t.Parallel()
	got := SanitizeName("a/b")
	if strings.Contains(got, "/") {
		t.Errorf("result should not contain '/', got %q", got)
	}
}

func TestSanitizeName_ReplacesBackslash(t *testing.T) {
	t.Parallel()
	got := SanitizeName(`a\b`)
	if strings.Contains(got, `\`) {
		t.Errorf("result should not contain backslash, got %q", got)
	}
}

func TestSanitizeName_TruncatesLongNames(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 100)
	got := SanitizeName(long)
	if len(got) > 50 {
		t.Errorf("result length %d exceeds 50", len(got))
	}
}

func TestSanitizeName_EmptyString_ReturnsUntitled(t *testing.T) {
	t.Parallel()
	got := SanitizeName("")
	if got != "untitled" {
		t.Errorf("got %q, want %q", got, "untitled")
	}
}

// TestSanitizeName_AllSpaces_BecomesHyphens verifies that an all-space input
// produces hyphens, not the "untitled" fallback (which only applies to empty strings).
func TestSanitizeName_AllSpaces_BecomesHyphens(t *testing.T) {
	t.Parallel()
	got := SanitizeName("   ")
	want := "---"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeName_SpecialCharactersKept(t *testing.T) {
	t.Parallel()
	got := SanitizeName("my-note-2024")
	if got != "my-note-2024" {
		t.Errorf("got %q, want %q", got, "my-note-2024")
	}
}

func TestGenerateNameOnDisk_ContainsTimestampAndName(t *testing.T) {
	t.Parallel()
	got := GenerateNameOnDisk("My Note")
	if !strings.Contains(got, "-") {
		t.Errorf("result should contain a dash separator, got %q", got)
	}
	if !strings.Contains(got, "my-note") {
		t.Errorf("result should contain sanitized name, got %q", got)
	}
}

func TestGenerateNameOnDisk_ReturnsNonEmptyString(t *testing.T) {
	t.Parallel()
	got := GenerateNameOnDisk("test")
	if got == "" {
		t.Error("GenerateNameOnDisk should not return an empty string")
	}
}

func TestGenerateNameOnDisk_EmptyName_UsesUntitled(t *testing.T) {
	t.Parallel()
	got := GenerateNameOnDisk("")
	if !strings.Contains(got, "untitled") {
		t.Errorf("empty name should produce 'untitled' in result, got %q", got)
	}
}
