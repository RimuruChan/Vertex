package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// CheckerSourceName is the checker that a built package leaves next to its
// test data. Its presence is what makes a snapshot a testlib snapshot.
const CheckerSourceName = "checker.cpp"

// Grader decides one test's verdict by comparing the program's output with the
// expected answer.
type Grader interface {
	// UsesSandbox reports whether grading runs a program inside the shared
	// sandbox workspace. When it does, the executor must copy the solution's
	// output out of the workspace before grading recycles it.
	UsesSandbox() bool
	Grade(ctx context.Context, inputPath, outputPath, answerPath string) (string, string)
}

// DiffGrader is the built-in normalized comparison. It never executes anything.
type DiffGrader struct{}

func (DiffGrader) UsesSandbox() bool { return false }

func (DiffGrader) Grade(_ context.Context, _, outputPath, answerPath string) (string, string) {
	decision, message, err := checker.CheckDiff(outputPath, answerPath)
	if err != nil {
		return verdict.SE, err.Error()
	}
	return decision, message
}

// TestlibGrader runs the compiled checker that came with the test data.
type TestlibGrader struct {
	runner      *checker.Runner
	checkerPath string
}

func (TestlibGrader) UsesSandbox() bool { return true }

func (g TestlibGrader) Grade(ctx context.Context, inputPath, outputPath, answerPath string) (string, string) {
	result, err := g.runner.Check(ctx, g.checkerPath, inputPath, outputPath, answerPath)
	if err != nil {
		return verdict.SE, err.Error()
	}
	return result.Verdict, result.Message
}

// Grader resolves the comparison strategy for one submission. A snapshot whose
// checker cannot be compiled fails the whole submission rather than silently
// falling back to a diff, because the two disagree by design.
func (e *Executor) Grader(ctx context.Context, testdataDir, kind string) (Grader, error) {
	if kind != "testlib" {
		return DiffGrader{}, nil
	}
	if e.checkerRunner == nil || e.checkerSource == nil {
		return nil, fmt.Errorf("this worker cannot run testlib checkers: testlib support is not configured")
	}
	source, err := os.ReadFile(filepath.Join(testdataDir, CheckerSourceName))
	if err != nil {
		return nil, fmt.Errorf("read packaged checker: %w", err)
	}
	path, err := e.checkerSource.Compile(ctx, source)
	if err != nil {
		return nil, err
	}
	return TestlibGrader{runner: e.checkerRunner, checkerPath: path}, nil
}
