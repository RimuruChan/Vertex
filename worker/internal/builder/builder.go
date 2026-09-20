package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
)

const (
	// maxTestBytes bounds one generated input or answer. It stays well under
	// the default sandbox workspace quota so a runaway generator is reported as
	// an output limit rather than as an opaque workspace failure.
	maxTestBytes = int64(32 << 20)
	// maxHeadBytes is how much of a test is echoed back to the author. Sample
	// tests are sent whole because the statement renders them verbatim.
	maxHeadBytes   = 512
	maxSampleBytes = 8 << 10
)

// Test outcome statuses reported to the authoring console.
const (
	StatusOK      = "ok"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Builder owns build staging and a sandbox environment factory.
// It must not be shared between concurrent builds.
type Builder struct {
	sandbox       *run.Client
	compiler      *compile.Compiler
	checker       *checker.Runner
	scratchRoot   string
	testlibPath   string
	testlibDigest string
}

func NewBuilder(
	sandbox *run.Client, compiler *compile.Compiler, scratchRoot, testlibPath string,
) (*Builder, error) {
	digest, err := fileDigest(testlibPath)
	if err != nil {
		return nil, fmt.Errorf("read testlib header: %w", err)
	}
	return &Builder{
		sandbox: sandbox, compiler: compiler,
		checker:     checker.NewRunner(sandbox, checker.DefaultLimits()),
		scratchRoot: scratchRoot, testlibPath: testlibPath, testlibDigest: digest,
	}, nil
}

// TestlibDigest reports which testlib revision this worker compiles against.
func (b *Builder) TestlibDigest() string { return b.testlibDigest }

// Build runs the whole pipeline. It returns a report for every terminal
// outcome: a package that fails validation is a completed build with
// Success=false, not an error. An error means the worker itself could not
// carry out the build and the job should be retried.
func (b *Builder) Build(ctx context.Context, job *Job, reporter Reporter) (*Report, error) {
	if job == nil {
		return &Report{Stage: StageCompile, ErrorMessage: "missing frozen check job"}, nil
	}
	return b.buildFrozen(ctx, job, reporter)
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
