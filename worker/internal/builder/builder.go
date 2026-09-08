package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
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
	if reporter == nil {
		reporter = NopReporter()
	}
	workspace, err := os.MkdirTemp(b.scratchRoot, "build-")
	if err != nil {
		return nil, fmt.Errorf("create build workspace: %w", err)
	}
	defer os.RemoveAll(workspace)

	report := &Report{Stage: StageCompile, Checker: "diff"}
	if job.Checker != nil {
		report.Checker = "testlib"
	}

	programs, failure := b.compilePackage(ctx, job, reporter)
	if failure != "" {
		report.ErrorMessage = failure
		return report, nil
	}

	report.Stage = StageGenerate
	tests, failure := b.materializeTests(ctx, job, programs, workspace, reporter)
	report.Tests = tests
	if failure != "" {
		report.ErrorMessage = failure
		return report, nil
	}

	report.Stage = StageSolutions
	solutions, failure := b.checkSolutions(ctx, job, programs, workspace, reporter)
	report.Solutions = solutions
	if failure != "" {
		report.ErrorMessage = failure
		return report, nil
	}

	report.Stage = StagePackage
	reporter.Stage(StagePackage, len(job.Tests), len(job.Tests))
	archive, err := buildArchive(workspace, len(job.Tests), job.Checker)
	if err != nil {
		report.ErrorMessage = "package assembly failed: " + err.Error()
		return report, nil
	}
	report.Archive = archive
	report.Success = true
	reporter.Logf("打包完成:%d 个测试点,%d 字节", len(job.Tests), len(archive))
	return report, nil
}

// compiledPackage holds every program the pipeline may need to run.
type compiledPackage struct {
	main       *program
	checker    *program
	validator  *program
	generators map[string]*program
	solutions  []*program
}

// compilePackage builds every source up front so an author sees all compile
// errors on the first failed stage instead of discovering them one test at a
// time.
func (b *Builder) compilePackage(ctx context.Context, job *Job, reporter Reporter) (*compiledPackage, string) {
	total := len(job.Generators) + len(job.Solutions)
	if job.Checker != nil {
		total++
	}
	if job.Validator != nil {
		total++
	}
	done := 0
	advance := func() {
		done++
		reporter.Stage(StageCompile, done, total)
	}

	result := &compiledPackage{generators: make(map[string]*program, len(job.Generators))}
	if job.Checker != nil {
		compiled, err := b.compileSource(ctx, *job.Checker, true)
		if err != nil {
			return nil, "checker 编译失败:\n" + err.Error()
		}
		result.checker = compiled
		reporter.Logf("checker %s 编译成功", job.Checker.Name)
		advance()
	}
	if job.Validator != nil {
		compiled, err := b.compileSource(ctx, *job.Validator, true)
		if err != nil {
			return nil, "validator 编译失败:\n" + err.Error()
		}
		result.validator = compiled
		reporter.Logf("validator %s 编译成功", job.Validator.Name)
		advance()
	}
	for _, generator := range job.Generators {
		compiled, err := b.compileSource(ctx, generator, true)
		if err != nil {
			return nil, "generator 编译失败:\n" + err.Error()
		}
		result.generators[generator.Name] = compiled
		reporter.Logf("generator %s 编译成功", generator.Name)
		advance()
	}
	for _, solution := range job.Solutions {
		compiled, err := b.compileSource(ctx, solution.SourceFile, false)
		if err != nil {
			return nil, "solution 编译失败:\n" + err.Error()
		}
		if solution.IsMain {
			result.main = compiled
		}
		result.solutions = append(result.solutions, compiled)
		reporter.Logf("solution %s 编译成功", solution.Name)
		advance()
	}
	if result.main == nil {
		return nil, "题目包缺少标程"
	}
	return result, ""
}

// materializeTests produces every input, validates it, and records the answer
// the model solution gives for it. A failure here stops the build: publishing
// half a test set would silently change what contestants are judged against.
func (b *Builder) materializeTests(
	ctx context.Context, job *Job, programs *compiledPackage, workspace string, reporter Reporter,
) ([]TestOutcome, string) {
	outcomes := make([]TestOutcome, 0, len(job.Tests))
	total := len(job.Tests)

	for position, test := range job.Tests {
		outcome := TestOutcome{
			Index: test.Index, Group: test.Group, Source: test.Source,
			Command: test.GenerateCmd, IsSample: test.IsSample, Points: test.Points,
			Status: StatusOK,
		}
		inputPath := filepath.Join(workspace, fmt.Sprintf("%d.in", test.Index))
		answerPath := filepath.Join(workspace, fmt.Sprintf("%d.out", test.Index))

		reporter.Stage(StageGenerate, position, total)
		if message := b.produceInput(ctx, job, test, programs, inputPath); message != "" {
			outcome.Status, outcome.Message = StatusFailed, message
			outcomes = append(outcomes, outcome)
			return outcomes, fmt.Sprintf("测试点 %d 生成失败: %s", test.Index, message)
		}
		outcome.InputBytes = fileSize(inputPath)

		if programs.validator != nil {
			reporter.Stage(StageValidate, position, total)
			if message := b.validateInput(ctx, job, programs, inputPath); message != "" {
				outcome.Status, outcome.Message = StatusFailed, message
				outcomes = append(outcomes, outcome)
				return outcomes, fmt.Sprintf("测试点 %d 未通过 validator: %s", test.Index, message)
			}
		}

		reporter.Stage(StageAnswer, position, total)
		answer, message := b.produceAnswer(ctx, job, programs, inputPath, answerPath)
		if message != "" {
			outcome.Status, outcome.Message = StatusFailed, message
			outcomes = append(outcomes, outcome)
			return outcomes, fmt.Sprintf("测试点 %d 标程运行失败: %s", test.Index, message)
		}
		outcome.TimeMs, outcome.MemoryKB = answer.timeMs, answer.memoryKB
		outcome.AnswerBytes = fileSize(answerPath)

		if programs.checker != nil {
			reporter.Stage(StageCheck, position, total)
			// Running the checker on the jury's own answer catches checkers
			// that cannot parse the very output format they are meant to grade.
			result, err := b.checker.Check(ctx, programs.checker.Path, inputPath, answerPath, answerPath)
			if err != nil {
				outcome.Status, outcome.Message = StatusFailed, err.Error()
				outcomes = append(outcomes, outcome)
				return outcomes, fmt.Sprintf("测试点 %d 自检失败: %s", test.Index, err.Error())
			}
			if result.Verdict != verdict.AC {
				outcome.Status, outcome.Message = StatusFailed, result.Message
				outcomes = append(outcomes, outcome)
				return outcomes, fmt.Sprintf(
					"测试点 %d 自检未通过:checker 对标程输出判定为 %s(%s)",
					test.Index, result.Verdict, result.Message)
			}
		}

		limit := maxHeadBytes
		if test.IsSample {
			limit = maxSampleBytes
		}
		outcome.InputHead = readHead(inputPath, limit)
		outcome.AnswerHead = readHead(answerPath, limit)
		outcomes = append(outcomes, outcome)
		reporter.Logf("测试点 %d 就绪(输入 %d B,答案 %d B,标程 %d ms)",
			test.Index, outcome.InputBytes, outcome.AnswerBytes, outcome.TimeMs)
	}
	reporter.Stage(StageCheck, total, total)
	return outcomes, ""
}

// produceInput writes the test input, either from the stored manual text or by
// running the generator command in the sandbox.
func (b *Builder) produceInput(
	ctx context.Context, job *Job, test TestSpec, programs *compiledPackage, inputPath string,
) string {
	if test.Source == SourceManual {
		if err := os.WriteFile(inputPath, normalizeText(test.InputData), 0o644); err != nil {
			return "写入手工输入失败: " + err.Error()
		}
		return ""
	}

	name, arguments, err := ParseGenerateCommand(test.GenerateCmd)
	if err != nil {
		return err.Error()
	}
	generator, ok := programs.generators[name]
	if !ok {
		return fmt.Sprintf("生成器 %q 不存在", name)
	}
	result, err := b.runProgram(ctx, execution{
		program:    generator,
		stdoutPath: inputPath,
		arguments:  arguments,
		timeLimit:  time.Duration(orDefault(job.Limits.GeneratorTimeMs, 30_000)) * time.Millisecond,
		memoryKB:   b.memoryKB(job),
		outputCap:  maxTestBytes,
	})
	if err != nil {
		return "生成器执行失败: " + err.Error()
	}
	if message := runFailure("生成器", result); message != "" {
		return message
	}
	// A generator that exits non-zero produced nothing usable, so its partial
	// stdout must never become a test input.
	if result.Meta.ExitCode != 0 {
		return fmt.Sprintf("生成器以退出码 %d 结束: %s",
			result.Meta.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return ""
}

// validateInput runs the validator with the input on standard input. testlib
// validators exit non-zero and explain themselves on stderr.
func (b *Builder) validateInput(ctx context.Context, job *Job, programs *compiledPackage, inputPath string) string {
	result, err := b.runProgram(ctx, execution{
		program:   programs.validator,
		stdinPath: inputPath,
		timeLimit: time.Duration(orDefault(job.Limits.ValidatorTimeMs, 30_000)) * time.Millisecond,
		memoryKB:  b.memoryKB(job),
		outputCap: maxTestBytes,
	})
	if err != nil {
		return "validator 执行失败: " + err.Error()
	}
	if message := runFailure("validator", result); message != "" {
		return message
	}
	if result.Meta.ExitCode != 0 {
		return strings.TrimSpace(result.Stderr)
	}
	return ""
}

type answerRun struct {
	timeMs   int
	memoryKB int
}

// produceAnswer runs the model solution to obtain the expected answer.
func (b *Builder) produceAnswer(
	ctx context.Context, job *Job, programs *compiledPackage, inputPath, answerPath string,
) (answerRun, string) {
	result, err := b.runProgram(ctx, execution{
		program:    programs.main,
		stdoutPath: answerPath,
		stdinPath:  inputPath,
		timeLimit:  time.Duration(orDefault(job.Limits.SolutionTimeMs, 60_000)) * time.Millisecond,
		memoryKB:   b.memoryKB(job),
		outputCap:  maxTestBytes,
	})
	if err != nil {
		return answerRun{}, "标程执行失败: " + err.Error()
	}
	if message := runFailure("标程", result); message != "" {
		return answerRun{}, message
	}
	if result.Meta.ExitCode != 0 {
		return answerRun{}, fmt.Sprintf("标程以退出码 %d 结束: %s",
			result.Meta.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return answerRun{
		timeMs:   int(result.Meta.Time * 1000),
		memoryKB: result.Meta.EffectiveMemoryKB(),
	}, ""
}

// checkSolutions runs every alternate solution against the built tests under
// the problem's own limits and compares the observed verdict with the one the
// author declared. This is the invocation (对拍) stage.
func (b *Builder) checkSolutions(
	ctx context.Context, job *Job, programs *compiledPackage, workspace string, reporter Reporter,
) ([]SolutionOutcome, string) {
	alternates := make([]int, 0, len(job.Solutions))
	for index, solution := range job.Solutions {
		if !solution.IsMain && solution.ExpectedVerdict != "" {
			alternates = append(alternates, index)
		}
	}
	if len(alternates) == 0 {
		return nil, ""
	}

	outcomes := make([]SolutionOutcome, 0, len(alternates))
	mismatched := make([]string, 0, len(alternates))
	for position, index := range alternates {
		reporter.Stage(StageSolutions, position, len(alternates))
		solution := job.Solutions[index]
		outcome := b.runSolution(ctx, job, programs, programs.solutions[index], solution, workspace)
		outcomes = append(outcomes, outcome)
		if !outcome.Matched {
			mismatched = append(mismatched, fmt.Sprintf("%s(期望 %s,实际 %s)",
				solution.Name, solution.ExpectedVerdict, outcome.ActualVerdict))
		}
		reporter.Logf("解 %s 判定为 %s(期望 %s)", solution.Name, outcome.ActualVerdict, solution.ExpectedVerdict)
	}
	reporter.Stage(StageSolutions, len(alternates), len(alternates))
	if len(mismatched) > 0 {
		return outcomes, "以下解的判定与预期不符: " + strings.Join(mismatched, "; ")
	}
	return outcomes, ""
}

// runSolution judges one alternate solution exactly the way the judge would,
// stopping at the first test that rejects it.
func (b *Builder) runSolution(
	ctx context.Context, job *Job, programs *compiledPackage,
	compiled *program, solution Solution, workspace string,
) SolutionOutcome {
	outcome := SolutionOutcome{
		Name: solution.Name, Language: solution.Language,
		ExpectedVerdict: solution.ExpectedVerdict, ActualVerdict: verdict.AC,
	}
	for _, test := range job.Tests {
		inputPath := filepath.Join(workspace, fmt.Sprintf("%d.in", test.Index))
		answerPath := filepath.Join(workspace, fmt.Sprintf("%d.out", test.Index))
		outputPath := filepath.Join(workspace, fmt.Sprintf("%d.%s.out", test.Index, solution.Name))

		result, err := b.runProgram(ctx, execution{
			program:    compiled,
			stdoutPath: outputPath,
			stdinPath:  inputPath,
			timeLimit:  time.Duration(job.TimeLimitMs) * time.Millisecond,
			memoryKB:   job.MemoryLimitKB,
			outputCap:  checker.MaxOutputBytes,
		})
		if err != nil {
			outcome.ActualVerdict, outcome.Message = verdict.SE, err.Error()
			outcome.FailedTest = test.Index
			break
		}
		if timeMs := int(result.Meta.Time * 1000); timeMs > outcome.MaxTimeMs {
			outcome.MaxTimeMs = timeMs
		}
		if memoryKB := result.Meta.EffectiveMemoryKB(); memoryKB > outcome.MaxMemoryKB {
			outcome.MaxMemoryKB = memoryKB
		}

		if limited := verdict.FromSandboxMeta(result.Meta); limited != "" {
			outcome.ActualVerdict = limited
			outcome.FailedTest = test.Index
			outcome.Message = result.Meta.ExitDescription()
			break
		}

		decision, message := b.compareOutput(ctx, programs, inputPath, outputPath, answerPath)
		os.Remove(outputPath)
		if decision != verdict.AC {
			outcome.ActualVerdict, outcome.Message = decision, message
			outcome.FailedTest = test.Index
			break
		}
	}
	outcome.Matched = verdictMatches(solution.ExpectedVerdict, outcome.ActualVerdict)
	return outcome
}

// compareOutput grades one output with the package checker, or with the
// built-in normalized diff when the package has none.
func (b *Builder) compareOutput(
	ctx context.Context, programs *compiledPackage, inputPath, outputPath, answerPath string,
) (string, string) {
	if programs.checker == nil {
		decision, message, err := checker.CheckDiff(outputPath, answerPath)
		if err != nil {
			return verdict.SE, err.Error()
		}
		return decision, message
	}
	result, err := b.checker.Check(ctx, programs.checker.Path, inputPath, outputPath, answerPath)
	if err != nil {
		return verdict.SE, err.Error()
	}
	return result.Verdict, result.Message
}

// verdictMatches compares a declared expectation with the observed verdict.
// "Any Rejection" accepts any non-Accepted outcome, which is what an author
// means by "this solution must fail somewhere".
func verdictMatches(expected, actual string) bool {
	switch expected {
	case "":
		return true
	case "Any Rejection":
		return actual != verdict.AC
	case "Presentation Error":
		return actual == verdict.WA
	default:
		return expected == actual
	}
}

// runFailure reports a sandbox-level limit breach. A plain non-zero exit is
// deliberately not a failure here: each caller decides what it means, because
// a validator uses a non-zero exit to reject an input.
func runFailure(role string, result *run.Result) string {
	limited := verdict.FromSandboxMeta(result.Meta)
	switch limited {
	case "":
		return ""
	case verdict.RE:
		if result.Meta.ExitSignal != 0 || result.Meta.Killed {
			return role + " 异常终止: " + result.Meta.ExitDescription()
		}
		return ""
	default:
		return role + " " + limited + ": " + result.Meta.ExitDescription()
	}
}

// memoryKB is the build-stage memory budget for jury programs, which is
// independent of the problem's own memory limit.
func (b *Builder) memoryKB(job *Job) int {
	if job != nil && job.Limits.MemoryLimitKB > 0 {
		return job.Limits.MemoryLimitKB
	}
	return 1 << 20
}

func orDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
