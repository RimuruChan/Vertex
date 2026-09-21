package domain

import "time"

type CompatibilityIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// Requirements retain unsupported imported semantics as part of the committed
// source, so saving/importing a package cannot silently enable wrong judging.
type MaterialRequirement struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
	Stage   string `json:"stage"`
}

type ImportOptions struct {
	Format      string `json:"format,omitempty"`
	TimeLimitMs int    `json:"timeLimitMs,omitempty"`
}

type ImportPlan struct {
	Scope       string               `json:"scope"`
	Format      string               `json:"format"`
	ArchiveHash string               `json:"archiveHash"`
	Tree        ContentTree          `json:"tree"`
	Issues      []CompatibilityIssue `json:"issues"`
	FileCount   int                  `json:"fileCount"`
	CanApply    bool                 `json:"canApply"`
}

type ImportReceipt struct {
	ID        string     `json:"id"`
	ETag      string     `json:"etag"`
	Plan      ImportPlan `json:"plan"`
	ExpiresAt time.Time  `json:"expiresAt"`
	Applied   bool       `json:"applied"`
}
type PackageExport struct {
	File     BlobRef              `json:"file"`
	Filename string               `json:"filename"`
	Format   string               `json:"format"`
	Issues   []CompatibilityIssue `json:"issues"`
}
