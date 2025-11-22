package util

import (
	"fmt"
	"strings"
	"time"
)

// SanitizeName removes characters that are problematic for file systems
func SanitizeName(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ToLower(name)
	// Basic sanitization
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		return "untitled"
	}
	return name
}

// GenerateNameOnDisk creates a filesystem-friendly name with a timestamp
func GenerateNameOnDisk(name string) string {
	now := time.Now().Unix()
	return fmt.Sprintf("%d-%s", now, SanitizeName(name))
}
