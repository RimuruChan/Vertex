package builder

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/statement"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

var frozenDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var frozenID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type frozenProgram struct {
	definition ProgramDefinition
	artifact   *compile.Artifact
}
type frozenBuild struct {
	downloadedBytes int64
	builder         *Builder
	job             *Job
	directory       string
	blobs           map[string]string
	programs        map[string]frozenProgram
	reporter        Reporter
	manifest        ArtifactManifest
	report          *Report
}

func (b *Builder) buildFrozen(ctx context.Context, job *Job, reporter Reporter) (*Report, error) {
	if err := validateFrozen(job); err != nil {
		return &Report{Stage: StageCompile, ErrorMessage: err.Error()}, nil
	}
	if reporter == nil {
		reporter = NopReporter()
	}
	directory, err := os.MkdirTemp(b.scratchRoot, "check-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	check := &frozenBuild{builder: b, job: job, directory: directory, blobs: map[string]string{}, programs: map[string]frozenProgram{}, reporter: reporter, manifest: ArtifactManifest{SchemaVersion: 1, Snapshot: *job.Check, Tests: []ArtifactTest{}}, report: &Report{Stage: StageCompile, Checker: job.Check.Metadata.Comparison.Kind}}
	languages := []string{}
	for _, program := range job.Check.Programs {
		languages = append(languages, program.Definition.Language)
	}
	nativePath, err := exec.LookPath("vertex-sandbox")
	if err != nil {
		return nil, err
	}
	nativeDigest, err := fileDigest(nativePath)
	if err != nil {
		return nil, err
	}
	texToolchain := ""
	if len(job.Check.Statements) > 0 {
		texToolchain, err = statement.Fingerprint(ctx)
		if err != nil {
			check.report.ErrorMessage = err.Error()
			return check.report, nil
		}
	}
	policy, err := json.Marshal(struct {
		Version string
		Testlib string
		Native  string
		Sandbox run.Policy
		Limits  Limits
		TeX     string
	}{CheckProtocol, b.testlibDigest, nativeDigest, b.sandbox.Policy, job.Limits, texToolchain})
	if err != nil {
		return nil, err
	}
	toolchain, err := compile.ToolchainFingerprint(ctx, languages, string(policy))
	if err != nil {
		check.report.ErrorMessage = "工具链不可用：" + err.Error()
		return check.report, nil
	}
	check.report.ToolchainKey = toolchain
	check.manifest.ToolchainKey = toolchain
	if err := check.compilePrograms(ctx); err != nil {
		check.report.ErrorMessage = err.Error()
		return check.report, nil
	}
	if err := check.materialize(ctx); err != nil {
		check.report.ErrorMessage = err.Error()
		return check.report, nil
	}
	if err := check.verifySolutions(ctx); err != nil {
		check.report.ErrorMessage = err.Error()
		return check.report, nil
	}
	if err := check.verifyValidators(ctx); err != nil {
		check.report.ErrorMessage = err.Error()
		return check.report, nil
	}
	if err := check.renderStatements(ctx); err != nil {
		check.report.ErrorMessage = err.Error()
		return check.report, nil
	}
	check.report.Stage = StagePackage
	archive, err := check.archive()
	if err != nil {
		check.report.ErrorMessage = err.Error()
		return check.report, nil
	}
	check.report.Archive = archive
	check.report.Success = true
	reporter.Stage(StagePackage, len(job.Check.Tests), len(job.Check.Tests))
	return check.report, nil
}

func validateFrozen(job *Job) error {
	s := job.Check
	if s == nil || job.FetchContent == nil || s.SchemaVersion != 1 || s.PolicyVersion != CheckProtocol || !frozenDigest.MatchString(s.TreeHash) || !frozenDigest.MatchString(s.DataHash) {
		return fmt.Errorf("invalid frozen build snapshot")
	}
	if s.Metadata.JudgeType != "normal" || s.Metadata.TimeLimitMs <= 0 || s.Metadata.TimeLimitMs > 3600000 || s.Metadata.MemoryLimitKB <= 0 || s.Metadata.MemoryLimitKB > 1<<30 {
		return fmt.Errorf("unsupported problem mode or invalid resource limits")
	}
	if len(s.Tests) == 0 || len(s.Tests) > 10000 || len(s.Programs) > 1000 {
		return fmt.Errorf("invalid build size")
	}
	if len(s.Validation) > 1000 || (len(s.Tests)+len(s.Validation))*len(s.Metadata.InputValidators) > 100000 {
		return fmt.Errorf("validator self-test matrix exceeds limits")
	}
	validationIDs := map[string]bool{}
	for _, item := range s.Validation {
		if !frozenID.MatchString(item.ID) || validationIDs[item.ID] || item.Input == nil {
			return fmt.Errorf("invalid validator self-test")
		}
		validationIDs[item.ID] = true
		switch item.Definition.Mode {
		case "invalid_input":
			if len(s.Metadata.InputValidators) == 0 || item.Answer != nil || item.Output != nil {
				return fmt.Errorf("invalid input self-test configuration")
			}
		case "valid_output", "invalid_output":
			if item.Answer == nil || item.Output == nil {
				return fmt.Errorf("output self-test files are missing")
			}
		default:
			return fmt.Errorf("unknown validator self-test mode")
		}
	}
	if len(s.Statements) > 20 {
		return fmt.Errorf("too many TeX statements")
	}
	statementIDs := map[string]bool{}
	for _, document := range s.Statements {
		if !frozenID.MatchString(document.ID) || statementIDs[document.ID] || len(document.Files) > 999 {
			return fmt.Errorf("invalid statement snapshot")
		}
		statementIDs[document.ID] = true
	}
	ids := map[string]bool{}
	solutions := 0
	for _, program := range s.Programs {
		if program.Definition.Role == "solution" {
			solutions++
		}
		if !frozenID.MatchString(program.ID) || ids[program.ID] {
			return fmt.Errorf("invalid program ID")
		}
		ids[program.ID] = true
		if len(program.Files) == 0 {
			return fmt.Errorf("program %s has no source files", program.Definition.Name)
		}
		if program.Definition.Role == "interactor" || program.Definition.Role == "static-validator" {
			return fmt.Errorf("program role %s is not executable by this check policy", program.Definition.Role)
		}
	}
	if solutions > 100000/len(s.Tests) {
		return fmt.Errorf("reference-solution matrix exceeds 100000 executions")
	}
	for _, test := range s.Tests {
		if !frozenID.MatchString(test.ID) || test.Definition.TimeLimitMs < 0 || test.Definition.TimeLimitMs > 3600000 || test.Definition.MemoryLimitKB < 0 || test.Definition.MemoryLimitKB > 1<<30 {
			return fmt.Errorf("invalid test definition")
		}
	}
	return nil
}

func (check *frozenBuild) content(ctx context.Context, ref BlobRef) (string, error) {
	if !frozenDigest.MatchString(ref.SHA256) || ref.Bytes < 0 || ref.Bytes > 64<<20 {
		return "", fmt.Errorf("invalid content reference")
	}
	if file, ok := check.blobs[ref.SHA256]; ok {
		return file, nil
	}
	if ref.Bytes > (64<<20)-check.downloadedBytes {
		return "", fmt.Errorf("build content exceeds 64 MiB")
	}
	reader, err := check.job.FetchContent(ctx, ref)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	name := filepath.Join(check.directory, "blob-"+ref.SHA256)
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, ref.Bytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size != ref.Bytes || hex.EncodeToString(hash.Sum(nil)) != ref.SHA256 {
		return "", fmt.Errorf("downloaded content does not match snapshot")
	}
	check.blobs[ref.SHA256] = name
	check.downloadedBytes += size
	return name, nil
}

func (check *frozenBuild) compilePrograms(ctx context.Context) error {
	for index, program := range check.job.Check.Programs {
		check.reporter.Stage(StageCompile, index, len(check.job.Check.Programs))
		files := map[string]string{}
		for _, source := range program.Files {
			if err := run.ValidateInputPath(source.Path); err != nil {
				return err
			}
			if _, ok := files[source.Path]; ok {
				return fmt.Errorf("duplicate program path")
			}
			file, err := check.content(ctx, source.Blob)
			if err != nil {
				return err
			}
			files[source.Path] = file
		}
		extension := compile.Extension{}
		if program.Definition.Language == "cpp" && files["testlib.h"] == "" {
			extension = checker.TestlibExtension(check.builder.testlibPath, check.builder.testlibDigest)
			if len(check.manifest.Dependencies) == 0 {
				ref, err := fileReference(check.builder.testlibPath)
				if err != nil {
					return err
				}
				if ref.SHA256 != check.builder.testlibDigest {
					return fmt.Errorf("testlib dependency changed after worker startup")
				}
				check.manifest.Dependencies = []FrozenFile{{ID: "testlib", Path: "testlib.h", Blob: ref}}
			}
		}
		artifact, result := check.builder.compiler.CompileFiles(ctx, program.Definition.Language, program.EntryPoint, files, extension)
		if !result.OK {
			return fmt.Errorf("%s 编译失败：%s", program.Definition.Name, result.Error)
		}
		check.programs[program.ID] = frozenProgram{definition: program.Definition, artifact: artifact}
		check.reporter.Logf("%s 编译完成", program.Definition.Name)
	}
	return nil
}

func (check *frozenBuild) execute(ctx context.Context, id, stdin, stdout string, args []string, timeMs, memoryKB int, exact ...bool) (*run.Result, error) {
	program, ok := check.programs[id]
	if !ok {
		return nil, fmt.Errorf("program %s not found", id)
	}
	config := compile.Supported[program.definition.Language]
	if timeMs <= 0 || memoryKB <= 0 {
		return nil, fmt.Errorf("invalid execution limits")
	}
	inputs := map[string]run.InputFile{}
	for name, file := range program.artifact.Files {
		inputs[name] = file
	}
	stdinName := ""
	if stdin != "" {
		stdinName = "__vertex_stdin"
		if _, ok := inputs[stdinName]; ok {
			return nil, fmt.Errorf("program uses reserved stdin file name")
		}
		inputs[stdinName] = run.InputFile{Path: stdin}
	}
	memory := int(float64(memoryKB)*config.MemFactor) + config.MemAddKB
	factor := config.TimeFactor
	if len(exact) > 0 && exact[0] {
		memory = memoryKB
		factor = 1
	}
	env, err := check.builder.sandbox.Create(ctx, run.EnvironmentPolicy{MemoryKB: memory, Processes: config.ProcAllow})
	if err != nil {
		return nil, err
	}
	defer env.Close()
	if err := env.PutFiles(ctx, inputs); err != nil {
		return nil, err
	}
	command := append([]string{}, program.artifact.Command...)
	command = append(command, program.definition.Arguments...)
	command = append(command, args...)
	cpu := time.Duration(float64(timeMs)*factor) * time.Millisecond
	return env.Run(ctx, run.Execution{Command: command, StdinFile: stdinName, StdoutPath: stdout, Limits: run.Limits{CPUTime: cpu, WallTime: cpu * 2, MemoryKB: memory, Processes: config.ProcAllow, OutputBytes: maxTestBytes}})
}

func (check *frozenBuild) compare(ctx context.Context, input, output, answer string) (checker.Verdict, error) {
	policy := check.job.Check.Metadata.Comparison
	if policy.Kind == "tokens" || policy.Kind == "exact" {
		return checker.CheckOutput(output, answer, policy)
	}
	program, ok := check.programs[check.job.Check.Metadata.OutputValidator]
	if !ok {
		return checker.Verdict{}, fmt.Errorf("output validator is missing")
	}
	return check.builder.checker.CheckProgram(ctx, checker.Program{Command: program.artifact.Command, Files: program.artifact.Files}, program.definition.Protocol, program.definition.Arguments, input, output, answer)
}

func (check *frozenBuild) materialize(ctx context.Context) error {
	totalBytes := int64(0)
	previewBytes := 128 << 10
	for index, test := range check.job.Check.Tests {
		number := index + 1
		definition := test.Definition
		outcome := TestOutcome{Index: number, Group: definition.Group, Source: definition.Input.Kind, IsSample: definition.IsSample, Status: StatusOK, Points: definition.Points}
		input, answer := filepath.Join(check.directory, fmt.Sprintf("%d.in", number)), filepath.Join(check.directory, fmt.Sprintf("%d.out", number))
		check.report.Stage = StageGenerate
		check.reporter.Stage(StageGenerate, index, len(check.job.Check.Tests))
		if definition.Input.Kind == "file" {
			if test.Input == nil {
				return fmt.Errorf("test %s has no input reference", definition.Name)
			}
			source, err := check.content(ctx, *test.Input)
			if err != nil {
				return err
			}
			if err := copyFile(source, input); err != nil {
				return err
			}
		} else if definition.Input.Kind == "generator" {
			result, err := check.execute(ctx, definition.Input.Generator, "", input, definition.Input.Arguments, check.job.Limits.GeneratorTimeMs, check.job.Limits.MemoryLimitKB)
			if err != nil {
				return err
			}
			if failure := executionFailure(result); failure != "" {
				return fmt.Errorf("%s 生成失败：%s", definition.Name, failure)
			}
		} else {
			return fmt.Errorf("unknown test input mode")
		}
		check.report.Stage = StageValidate
		for _, id := range check.job.Check.Metadata.InputValidators {
			check.reporter.Stage(StageValidate, index, len(check.job.Check.Tests))
			program, ok := check.programs[id]
			if !ok {
				return fmt.Errorf("input validator missing")
			}
			result, err := check.execute(ctx, id, input, "", nil, check.job.Limits.ValidatorTimeMs, check.job.Limits.MemoryLimitKB)
			if err != nil {
				return err
			}
			if result.Meta == nil {
				return fmt.Errorf("validator returned no metadata")
			}
			accepted, err := checker.ValidatorAccepted(*result.Meta, program.definition.Protocol)
			if err != nil {
				return err
			}
			if !accepted {
				return fmt.Errorf("%s 未通过 %s：%s", definition.Name, program.definition.Name, result.Stderr)
			}
		}
		check.report.Stage = StageAnswer
		check.reporter.Stage(StageAnswer, index, len(check.job.Check.Tests))
		if definition.Answer.Kind == "file" {
			if test.Answer == nil {
				return fmt.Errorf("test %s has no answer reference", definition.Name)
			}
			source, err := check.content(ctx, *test.Answer)
			if err != nil {
				return err
			}
			if err := copyFile(source, answer); err != nil {
				return err
			}
		} else if definition.Answer.Kind == "solution" {
			id := definition.Answer.Solution
			if id == "" {
				id = check.job.Check.Metadata.MainSolution
			}
			result, err := check.execute(ctx, id, input, answer, nil, check.job.Limits.SolutionTimeMs, check.job.Limits.MemoryLimitKB)
			if err != nil {
				return err
			}
			if failure := executionFailure(result); failure != "" {
				return fmt.Errorf("%s 生成答案失败：%s", definition.Name, failure)
			}
		} else {
			return fmt.Errorf("unknown test answer mode")
		}
		decision, err := check.compare(ctx, input, answer, answer)
		if err != nil {
			return err
		}
		if decision.Verdict != verdict.AC {
			return fmt.Errorf("%s 答案自检失败：%s %s", definition.Name, decision.Message, decision.JudgeMessage)
		}
		inRef, err := fileReference(input)
		if err != nil {
			return err
		}
		answerRef, err := fileReference(answer)
		if err != nil {
			return err
		}
		check.manifest.Tests = append(check.manifest.Tests, ArtifactTest{ID: test.ID, Input: inRef, Answer: answerRef})
		totalBytes += inRef.Bytes + answerRef.Bytes
		if totalBytes > 64<<20 {
			return fmt.Errorf("materialized tests exceed 64 MiB")
		}
		outcome.InputBytes = inRef.Bytes
		outcome.AnswerBytes = answerRef.Bytes
		if definition.IsSample {
			limit := min(maxSampleBytes, previewBytes/2)
			if limit > 0 {
				outcome.InputHead = readHead(input, limit)
				outcome.AnswerHead = readHead(answer, limit)
			}
			previewBytes -= len(outcome.InputHead) + len(outcome.AnswerHead)
			outcome.HeadsTruncated = inRef.Bytes > int64(limit) || answerRef.Bytes > int64(limit)
		}
		check.report.Tests = append(check.report.Tests, outcome)
	}
	return nil
}

func (check *frozenBuild) verifySolutions(ctx context.Context) error {
	check.report.Stage = StageSolutions
	messageBytes := 128 << 10
	for _, source := range check.job.Check.Programs {
		if source.Definition.Role != "solution" {
			continue
		}
		outcome := SolutionOutcome{Name: source.Definition.Name, Language: source.Definition.Language, ExpectedVerdict: strings.Join(source.Definition.ExpectedVerdicts, ", "), ActualVerdict: verdict.AC}
		observed := []string{}
		for index, test := range check.job.Check.Tests {
			check.reporter.Stage(StageSolutions, index, len(check.job.Check.Tests))
			timeMs, memoryKB := test.Definition.TimeLimitMs, test.Definition.MemoryLimitKB
			if timeMs == 0 {
				timeMs = check.job.Check.Metadata.TimeLimitMs
			}
			if memoryKB == 0 {
				memoryKB = check.job.Check.Metadata.MemoryLimitKB
			}
			output := filepath.Join(check.directory, fmt.Sprintf("solution-%s-%d.out", source.ID, index+1))
			result, err := check.execute(ctx, source.ID, filepath.Join(check.directory, fmt.Sprintf("%d.in", index+1)), output, nil, timeMs, memoryKB, check.job.Check.Metadata.ResourceMode == "exact")
			if err != nil {
				return err
			}
			actual := verdict.FromSandboxMeta(result.Meta)
			if actual == "" {
				decision, err := check.compare(ctx, filepath.Join(check.directory, fmt.Sprintf("%d.in", index+1)), output, filepath.Join(check.directory, fmt.Sprintf("%d.out", index+1)))
				if err != nil {
					return err
				}
				actual = decision.Verdict
				if decision.Verdict != verdict.AC && outcome.Message == "" && messageBytes > 0 {
					outcome.Message = decision.Message
					if decision.JudgeMessage != "" {
						outcome.Message += "\n" + decision.JudgeMessage
					}
					if len(outcome.Message) > messageBytes {
						outcome.Message = outcome.Message[:messageBytes]
					}
					messageBytes -= len(outcome.Message)
				}
			}
			if actual == verdict.SE {
				return fmt.Errorf("判定参考解 %s 时校验器发生错误：%s", source.Definition.Name, outcome.Message)
			}
			observed = append(observed, actual)
			if verdictPriority(actual) > verdictPriority(outcome.ActualVerdict) {
				outcome.ActualVerdict = actual
				outcome.FailedTest = index + 1
			}
			if result.Meta != nil {
				outcome.Cases = append(outcome.Cases, SolutionCaseOutcome{Index: index + 1, Verdict: actual, TimeMs: int(result.Meta.Time * 1000), MemoryKB: result.Meta.MaxRSS})
				outcome.MaxTimeMs = max(outcome.MaxTimeMs, int(result.Meta.Time*1000))
				outcome.MaxMemoryKB = max(outcome.MaxMemoryKB, result.Meta.MaxRSS)
			}
			if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		outcome.Matched = expectedSolution(source.Definition.ExpectedVerdicts, observed)
		if source.ID == check.job.Check.Metadata.MainSolution {
			outcome.Matched = expectedSolution([]string{verdict.AC}, observed)
		}
		check.report.Solutions = append(check.report.Solutions, outcome)
		if !outcome.Matched {
			return fmt.Errorf("参考解 %s 的结果与预期不符", source.Definition.Name)
		}
	}
	return nil
}

func verdictPriority(value string) int {
	switch value {
	case verdict.SE:
		return 7
	case verdict.RE:
		return 6
	case verdict.MLE:
		return 5
	case verdict.TLE:
		return 4
	case verdict.OLE:
		return 3
	case verdict.WA:
		return 2
	case verdict.AC:
		return 0
	default:
		return 1
	}
}

func executionFailure(result *run.Result) string {
	if result == nil || result.Meta == nil {
		return "execution metadata missing"
	}
	if value := verdict.FromSandboxMeta(result.Meta); value != "" {
		return value + ": " + result.Stderr
	}
	return ""
}

func expectedSolution(expected, actual []string) bool {
	for _, want := range expected {
		found := false
		valid := true
		for _, got := range actual {
			if got == verdict.SE {
				valid = false
			}
			if got == want || want == "Any Rejection" && got != verdict.AC {
				found = true
			}
			switch want {
			case verdict.AC:
				if got != verdict.AC {
					valid = false
				}
			case verdict.WA:
				if got != verdict.AC && got != verdict.WA {
					valid = false
				}
			case verdict.TLE:
				if got != verdict.AC && got != verdict.WA && got != verdict.TLE {
					valid = false
				}
			}
		}
		if found && valid {
			return true
		}
	}
	return false
}

func fileReference(name string) (BlobRef, error) {
	file, err := os.Open(name)
	if err != nil {
		return BlobRef{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, maxTestBytes+1))
	if err != nil {
		return BlobRef{}, err
	}
	if size > maxTestBytes {
		return BlobRef{}, fmt.Errorf("test file exceeds build limit")
	}
	return BlobRef{SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: size}, nil
}

func (check *frozenBuild) archive() ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	manifest, err := json.Marshal(check.manifest)
	if err != nil {
		return nil, err
	}
	if len(manifest) > 8<<20 {
		return nil, fmt.Errorf("artifact manifest exceeds 8 MiB")
	}
	totalBytes := int64(len(manifest))
	for _, test := range check.manifest.Tests {
		totalBytes += test.Input.Bytes + test.Answer.Bytes
	}
	for _, program := range check.job.Check.Programs {
		for _, source := range program.Files {
			totalBytes += source.Blob.Bytes
		}
	}
	for _, dependency := range check.manifest.Dependencies {
		totalBytes += dependency.Blob.Bytes
	}
	for _, document := range check.manifest.Statements {
		totalBytes += document.PDF.Bytes
	}
	if totalBytes > 64<<20 {
		return nil, fmt.Errorf("build artifact exceeds 64 MiB")
	}
	entry, err := writer.Create("artifact.json")
	if err != nil {
		return nil, err
	}
	if _, err := entry.Write(manifest); err != nil {
		return nil, err
	}
	for index := range check.manifest.Tests {
		for _, suffix := range []string{".in", ".out"} {
			name := fmt.Sprintf("%d%s", index+1, suffix)
			if err := addFile(writer, name, filepath.Join(check.directory, name)); err != nil {
				return nil, err
			}
		}
	}
	names := map[string]string{}
	for _, document := range check.manifest.Statements {
		names["statements/"+document.ID+".pdf"] = filepath.Join(check.directory, "statement-"+document.ID+".pdf")
	}
	for _, dependency := range check.manifest.Dependencies {
		names["dependencies/"+dependency.Path] = check.builder.testlibPath
	}
	for _, program := range check.job.Check.Programs {
		for _, source := range program.Files {
			names["programs/"+program.ID+"/"+source.Path] = check.blobs[source.Blob.SHA256]
		}
	}
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if err := addFile(writer, name, names[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if buffer.Len() > 64<<20 {
		return nil, fmt.Errorf("compressed build artifact exceeds 64 MiB")
	}
	return buffer.Bytes(), nil
}
