package domain

// SearchMatch represents one note search result with contextual snippet data.
type SearchMatch struct {
	Note        Note   `json:"note"`
	Line        int    `json:"line"`
	Snippet     string `json:"snippet"`
	SourceLabel string `json:"sourceLabel"`
}
