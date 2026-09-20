package domain

import (
	"context"
	"time"
)

// WorkerBuildRepository contains only frozen-job leasing and result storage.
// Editable materials and publication are separate Workbench use cases.
type WorkerBuildRepository interface {
	WithArtifactStorage(context.Context, string, func(context.Context) error) error
	Claim(context.Context, string, time.Duration) (*Build, *CheckInput, error)
	Progress(context.Context, Progress, time.Duration) error
	ResolvePackageTarget(context.Context, string, string, string) (string, error)
	RecordPackage(context.Context, string, string, string, string, PackageUpload) error
	Complete(context.Context, BuildResult, string) error
}
type ArtifactPublisher interface {
	Publish(string, []byte) (*PackageUpload, error)
}
