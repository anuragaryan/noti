package domain

import "time"

// Note represents a note entry
type Note struct {
	ID                  string    `json:"id"`
	Title               string    `json:"title"`
	FileStem            string    `json:"fileStem"`
	FolderID            string    `json:"folderId"`
	TranscriptActivated bool      `json:"transcriptActivated"`
	MarkdownContent     string    `json:"markdownContent"`
	TranscriptContent   string    `json:"transcriptContent"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	Order               int       `json:"order"`
}
