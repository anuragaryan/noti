package domain

import "time"

// Folder represents a folder/category
type Folder struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	NameOnDisk string    `json:"nameOnDisk"`
	ParentID   string    `json:"parentId"`
	CreatedAt  time.Time `json:"createdAt"`
	Order      int       `json:"order"`
}

// FolderStructure represents the folder/note organization
type FolderStructure struct {
	Folders []Folder `json:"folders"`
	Notes   []Note   `json:"notes"`
}
