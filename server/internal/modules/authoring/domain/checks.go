package domain

import (
	"context"
	"time"
)

type checkProtocolKey struct{}

func WithCheckProtocol(ctx context.Context, protocol string) context.Context {
	return context.WithValue(ctx, checkProtocolKey{}, protocol)
}
func AcceptsCheckProtocol(ctx context.Context) bool {
	return ctx.Value(checkProtocolKey{}) == CheckPolicyVersion
}

type CheckSelection struct {
	Revision int64  `json:"revision,omitempty"`
	ETag     string `json:"etag,omitempty"`
}

func (selection CheckSelection) Validate() error {
	if selection.Revision < 0 || selection.Revision > 0 && selection.ETag != "" || selection.Revision == 0 && selection.ETag == "" {
		return InvalidInput("select a shared revision or an exact working-copy token")
	}
	return nil
}

type CheckRun struct {
	Validation       []ValidationOutcome `json:"validation,omitempty"`
	Statements       []StatementPreview  `json:"statements,omitempty"`
	MatchingRevision *int64              `json:"matchingRevision,omitempty"`
	ID               string              `json:"id"`
	TreeHash         string              `json:"treeHash"`
	Revision         *int64              `json:"revision,omitempty"`
	DataHash         string              `json:"dataHash"`
	PolicyVersion    string              `json:"policyVersion"`
	ToolchainKey     string              `json:"toolchainKey,omitempty"`
	State            string              `json:"state"`
	Stage            string              `json:"stage"`
	ProgressDone     int                 `json:"progressDone"`
	ProgressTotal    int                 `json:"progressTotal"`
	Log              string              `json:"log,omitempty"`
	ErrorMessage     string              `json:"errorMessage,omitempty"`
	Tests            []TestOutcome       `json:"tests,omitempty"`
	Solutions        []SolutionOutcome   `json:"solutions,omitempty"`
	PackageCases     int                 `json:"packageCases"`
	CreatedAt        time.Time           `json:"createdAt"`
	StartedAt        *time.Time          `json:"startedAt,omitempty"`
	FinishedAt       *time.Time          `json:"finishedAt,omitempty"`
}
type StatementPreview struct {
	ID       string `json:"id"`
	Language string `json:"language"`
}
