package content

import "time"

type Editorial struct {
	ID         string
	ProblemID  string
	AuthorID   *string
	AuthorName string
	Title      string
	ContentMD  string
	Visibility string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DiscussionPost struct {
	ID          int64
	ProblemID   *string
	EditorialID *string
	ContestID   *string
	AuthorID    *string
	AuthorName  string
	ContentMD   string
	ParentID    *int64
	CreatedAt   time.Time
}
