package domain

import "time"

type LibraryQuery struct {
	Keyword, Visibility, Status string
	Limit, Offset               int
}
type LibraryItem struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Source            string    `json:"source"`
	Visibility        string    `json:"visibility"`
	OwnerName         string    `json:"ownerName"`
	CanEdit           bool      `json:"canEdit"`
	CanPublish        bool      `json:"canPublish"`
	HasCopy           bool      `json:"hasCopy"`
	HasChanges        bool      `json:"hasChanges"`
	HasConflict       bool      `json:"hasConflict"`
	BaseRevision      int64     `json:"baseRevision"`
	HeadRevision      int64     `json:"headRevision"`
	PublishedVersion  int       `json:"publishedVersion"`
	PublishedRevision int64     `json:"publishedRevision"`
	CheckID           string    `json:"checkId,omitempty"`
	CheckState        string    `json:"checkState,omitempty"`
	CheckMatches      bool      `json:"checkMatches"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
type LibraryPage struct {
	Items []LibraryItem `json:"items"`
	Total int           `json:"total"`
}
