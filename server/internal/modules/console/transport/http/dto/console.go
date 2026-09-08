// Package dto maps the administration console onto the HTTP wire contract.
package dto

import (
	"time"

	consoledomain "github.com/RimuruChan/Vertex/server/internal/modules/console/domain"
)

// ---------- dashboard ----------

type VerdictCountResponse struct {
	Verdict string `json:"verdict"`
	Count   int    `json:"count"`
}

type StatsResponse struct {
	Users            int `json:"users"`
	UsersToday       int `json:"usersToday"`
	Problems         int `json:"problems"`
	PublicProblems   int `json:"publicProblems"`
	Submissions      int `json:"submissions"`
	SubmissionsToday int `json:"submissionsToday"`
	Contests         int `json:"contests"`
	RunningContests  int `json:"runningContests"`
	Editorials       int `json:"editorials"`
	ProblemSets      int `json:"problemSets"`
	// Judge queue health.
	QueuedJobs       int                    `json:"queuedJobs"`
	RunningJobs      int                    `json:"runningJobs"`
	DeadJobs         int                    `json:"deadJobs"`
	OldestQueued     *time.Time             `json:"oldestQueued,omitempty"`
	ActiveWorkers    int                    `json:"activeWorkers"`
	VerdictBreakdown []VerdictCountResponse `json:"verdictBreakdown"`
}

func FromStats(value consoledomain.Stats) StatsResponse {
	response := StatsResponse{
		Users: value.Users, UsersToday: value.UsersToday,
		Problems: value.Problems, PublicProblems: value.PublicProblems,
		Submissions: value.Submissions, SubmissionsToday: value.SubmissionsToday,
		Contests: value.Contests, RunningContests: value.RunningContests,
		Editorials: value.Editorials, ProblemSets: value.ProblemSets,
		QueuedJobs: value.QueuedJobs, RunningJobs: value.RunningJobs,
		DeadJobs: value.DeadJobs, OldestQueued: value.OldestQueued,
		ActiveWorkers:    value.ActiveWorkers,
		VerdictBreakdown: make([]VerdictCountResponse, 0, len(value.VerdictBreakdown)),
	}
	for _, item := range value.VerdictBreakdown {
		response.VerdictBreakdown = append(response.VerdictBreakdown, VerdictCountResponse{Verdict: item.Verdict, Count: item.Count})
	}
	return response
}

// ---------- accounts ----------

type AccountResponse struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	Role            string     `json:"role" enums:"user,admin"`
	Rating          int        `json:"rating"`
	CreatedAt       time.Time  `json:"createdAt"`
	Disabled        bool       `json:"disabled"`
	DisabledAt      *time.Time `json:"disabledAt,omitempty"`
	DisabledReason  string     `json:"disabledReason,omitempty"`
	SubmissionCount int        `json:"submissionCount"`
	SolvedCount     int        `json:"solvedCount"`
}

// AccountUpdateRequest carries only the fields the administrator changed.
// Omitted fields are left as they are.
type AccountUpdateRequest struct {
	Role     *string `json:"role,omitempty" enums:"user,admin"`
	Rating   *int    `json:"rating,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
	Reason   string  `json:"reason,omitempty"`
}

func (request AccountUpdateRequest) Update() consoledomain.AccountUpdate {
	return consoledomain.AccountUpdate{
		Role: request.Role, Rating: request.Rating,
		Disabled: request.Disabled, Reason: request.Reason,
	}
}

func FromAccount(value consoledomain.AccountSummary) AccountResponse {
	return AccountResponse{
		ID: value.ID, Username: value.Username, Email: value.Email, Role: value.Role,
		Rating: value.Rating, CreatedAt: value.CreatedAt, Disabled: value.Disabled(),
		DisabledAt: value.DisabledAt, DisabledReason: value.DisabledReason,
		SubmissionCount: value.SubmissionCount, SolvedCount: value.SolvedCount,
	}
}

func FromAccounts(values []consoledomain.AccountSummary) []AccountResponse {
	result := make([]AccountResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromAccount(value))
	}
	return result
}

// ---------- tags ----------

type TagCatalogResponse struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ProblemCount int    `json:"problemCount"`
}

type TagRenameRequest struct {
	Name string `json:"name" binding:"required"`
}

type TagMergeRequest struct {
	TargetID int64 `json:"targetId" binding:"required"`
}

func FromTag(value consoledomain.Tag) TagCatalogResponse {
	return TagCatalogResponse{ID: value.ID, Name: value.Name, ProblemCount: value.ProblemCount}
}

func FromTags(values []consoledomain.Tag) []TagCatalogResponse {
	result := make([]TagCatalogResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromTag(value))
	}
	return result
}

// ---------- announcements ----------

type AnnouncementResponse struct {
	ID         string    `json:"id"`
	PublicID   string    `json:"publicId"`
	Title      string    `json:"title"`
	ContentMD  string    `json:"contentMd"`
	Pinned     bool      `json:"pinned"`
	Published  bool      `json:"published"`
	AuthorName string    `json:"authorName,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type AnnouncementUpsertRequest struct {
	Title     string `json:"title" binding:"required"`
	ContentMD string `json:"contentMd,omitempty"`
	Pinned    bool   `json:"pinned,omitempty"`
	Published *bool  `json:"published,omitempty"`
}

// Input defaults published to true: an announcement written in the console is
// normally meant to go out immediately.
func (request AnnouncementUpsertRequest) Input() consoledomain.AnnouncementInput {
	published := true
	if request.Published != nil {
		published = *request.Published
	}
	return consoledomain.AnnouncementInput{
		Title: request.Title, ContentMD: request.ContentMD,
		Pinned: request.Pinned, Published: published,
	}
}

func FromAnnouncement(value consoledomain.Announcement) AnnouncementResponse {
	return AnnouncementResponse{
		ID: value.ID, PublicID: value.PublicID, Title: value.Title, ContentMD: value.ContentMD,
		Pinned: value.Pinned, Published: value.Published, AuthorName: value.AuthorName,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func FromAnnouncements(values []consoledomain.Announcement) []AnnouncementResponse {
	result := make([]AnnouncementResponse, 0, len(values))
	for _, value := range values {
		result = append(result, FromAnnouncement(value))
	}
	return result
}
