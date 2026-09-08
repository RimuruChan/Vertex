package checker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// testlib exit codes. Anything else is treated as a checker crash, which is a
// jury-side fault rather than a contestant's wrong answer.
const (
	exitOK            = 0
	exitWrongAnswer   = 1
	exitPresentation  = 2
	exitFail          = 3
	exitDirt          = 4
	exitPoints        = 7
	exitUnexpectedEOF = 8
)

// Box file names used while running a checker. They are single path
// components because the sandbox only accepts one-component names.
const (
	boxCheckerName = "checker"
	boxInputName   = "checker_input"
	boxOutputName  = "checker_output"
	boxAnswerName  = "checker_answer"
)

// maxCheckerMessageBytes bounds the comment shown to the contestant.
const maxCheckerMessageBytes = 4096

// Verdict is the outcome of running a checker over one test.
type Verdict struct {
	Verdict string
	Message string
}

// Runner executes a compiled testlib checker inside the sandbox. The checker
// is authored by the jury, but it is still untrusted code from the kernel's
// point of view, so it runs under the same isolation as a submission.
type Runner struct {
	sandbox *run.Client
	limits  Limits
}

// Limits is the resource budget for one checker invocation.
type Limits struct {
	CPUTime   time.Duration
	MemoryKB  int
	Processes int
}

func DefaultLimits() Limits {
	return Limits{CPUTime: 10 * time.Second, MemoryKB: 512 * 1024, Processes: 4}
}

func NewRunner(sandbox *run.Client, limits Limits) *Runner {
	if limits.CPUTime <= 0 || limits.MemoryKB <= 0 || limits.Processes <= 0 {
		limits = DefaultLimits()
	}
	return &Runner{sandbox: sandbox, limits: limits}
}

// Check runs `checker input output answer` and maps the exit code to a
// verdict. All three paths are host-side files; they are copied into the
// workspace because the checker cannot reach outside it.
func (r *Runner) Check(ctx context.Context, checkerPath, inputPath, outputPath, answerPath string) (Verdict, error) {
	env, err := r.sandbox.Create(ctx, run.EnvironmentPolicy{MemoryKB: r.limits.MemoryKB, Processes: r.limits.Processes})
	if err != nil {
		return Verdict{}, err
	}
	defer env.Close()
	if err := env.PutFiles(ctx, map[string]run.InputFile{
		boxCheckerName: {Path: checkerPath, Executable: true},
		boxInputName:   {Path: inputPath}, boxOutputName: {Path: outputPath}, boxAnswerName: {Path: answerPath},
	}); err != nil {
		return Verdict{}, err
	}
	result, err := env.Run(ctx, run.Execution{
		Command: []string{"./" + boxCheckerName, boxInputName, boxOutputName, boxAnswerName},
		Limits: run.Limits{
			CPUTime:     r.limits.CPUTime,
			WallTime:    r.limits.CPUTime * 2,
			MemoryKB:    r.limits.MemoryKB,
			Processes:   r.limits.Processes,
			OutputBytes: MaxOutputBytes,
		},
	})
	if err != nil {
		return Verdict{}, fmt.Errorf("run checker: %w", err)
	}

	message := readCheckerComment(result.Stderr, result.Stdout)
	// A checker that hits its own limits says nothing about the submission, so
	// it is reported as a system error rather than as a wrong answer.
	if limited := verdict.FromSandboxMeta(result.Meta); limited != "" && result.Meta.Status != "RE" {
		return Verdict{
			Verdict: verdict.SE,
			Message: "checker " + limited + ": " + result.Meta.ExitDescription(),
		}, nil
	}

	switch result.Meta.ExitCode {
	case exitOK:
		return Verdict{Verdict: verdict.AC, Message: message}, nil
	case exitWrongAnswer:
		return Verdict{Verdict: verdict.WA, Message: message}, nil
	case exitPresentation, exitDirt, exitUnexpectedEOF:
		// Vertex has no separate presentation verdict; testlib's presentation
		// family is reported as a wrong answer with the checker's comment.
		return Verdict{Verdict: verdict.WA, Message: message}, nil
	case exitPoints:
		// Partial scoring is not part of the judge contract yet: any non-zero
		// score short of a full accept is a wrong answer.
		return Verdict{Verdict: verdict.WA, Message: message}, nil
	case exitFail:
		return Verdict{Verdict: verdict.SE, Message: "checker failed: " + message}, nil
	default:
		return Verdict{
			Verdict: verdict.SE,
			Message: fmt.Sprintf("checker exited with %d: %s", result.Meta.ExitCode, message),
		}, nil
	}
}

// readCheckerComment prefers stderr, where testlib writes its verdict comment,
// and falls back to stdout for checkers that print there instead.
func readCheckerComment(stderr, stdout string) string {
	for _, message := range []string{stderr, stdout} {
		data := []byte(message)
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		if len(text) > maxCheckerMessageBytes {
			text = text[:maxCheckerMessageBytes] + "…"
		}
		return text
	}
	return ""
}

// TestlibExtension builds the compile extension that puts testlib.h next to the
// source inside the workspace. Using a quoted include with the header in the
// working directory means no include path ever points outside the sandbox.
func TestlibExtension(testlibPath, testlibDigest string) compile.Extension {
	return compile.Extension{
		Files: map[string]string{"testlib.h": testlibPath},
		Args:  []string{"-I."},
		Salt:  "testlib:" + testlibDigest,
	}
}
