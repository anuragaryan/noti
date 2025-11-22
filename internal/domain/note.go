package domain

import "time"

// Note represents a note entry
type Note struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	NameOnDisk string    `json:"nameOnDisk"`
	FolderID   string    `json:"folderId"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Order      int       `json:"order"`
}
