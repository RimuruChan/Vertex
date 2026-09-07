package content

import "time"

// Editorial status values.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
)

// Editorial visibility values.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// Editorial is one solution write-up for a problem.
type Editorial struct {
	DomainID        string
	Permissions     Permissions
	PublicID        string
	ProblemPublicID string
	ID              string
	ProblemID       string
	// ProblemTitle is filled by the read paths that list across problems.
	ProblemTitle string
	AuthorID     *string
	AuthorName   string
	Title        string
	ContentMD    string
	Visibility   string
	Status       string
	// SolvedOnly hides the body from readers who have not solved the problem.
	// It is the anti-spoiler switch: the entry stays listed so nobody wonders
	// whether an editorial exists, but the text is withheld.
	SolvedOnly bool
	VoteCount  int
	// Voted reports whether the current viewer upvoted this editorial.
	Voted bool
	// Locked is true when SolvedOnly withheld the body from this viewer.
	Locked    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EditorialSummary is the list read model. It intentionally has no ContentMD;
// clients load the body from the detail endpoint only when it is opened.
type EditorialSummary struct {
	DomainID        string
	Permissions     Permissions
	PublicID        string
	ProblemPublicID string
	ID              string
	ProblemID       string
	ProblemTitle    string
	AuthorID        *string
	AuthorName      string
	Title           string
	Visibility      string
	Status          string
	SolvedOnly      bool
	VoteCount       int
	Voted           bool
	Locked          bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// DiscussionPost is one comment. Exactly one of the scope IDs is set, matching
// the database check constraint.
type DiscussionPost struct {
	DomainID    string
	Permissions Permissions
	ID          int64
	ProblemID   *string
	EditorialID *string
	AuthorID    *string
	AuthorName  string
	ContentMD   string
	ParentID    *int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Edited reports whether the post was changed after it was written.
func (p *DiscussionPost) Edited() bool { return p.UpdatedAt.After(p.CreatedAt.Add(time.Second)) }

// EditorialFilters narrows a cross-problem editorial listing.
type EditorialFilters struct {
	ProblemID string
	AuthorID  string
	Keyword   string
	ViewerID  string
	Admin     bool // Legacy caller hint; persistence resolves current domain rights.
	// Sort selects the ordering: "recent" (default) or "votes".
	Sort   string
	Limit  int
	Offset int
}

// EditorialInput is the editable part of an editorial.
type EditorialInput struct {
	ProblemID  string
	Title      string
	ContentMD  string
	Visibility string
	Status     string
	SolvedOnly bool
}
