package authoring

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Languages accepted for package sources. testlib is a C++ header, so every
// role that links against it is restricted to C++; generators and solutions
// may use any judged language.
var (
	testlibLanguages  = map[string]bool{"cpp": true}
	generatorLanguage = map[string]bool{"cpp": true, "python": true}
	solutionLanguages = map[string]bool{"c": true, "cpp": true, "python": true}
)

var (
	fileNamePattern     = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)
	generatorArgPattern = regexp.MustCompile(`^[-A-Za-z0-9_.,=:+/@\[\]]{1,64}$`)
	languagePattern     = regexp.MustCompile(`^[a-z]{2}(-[A-Za-z]{2,8})?$`)
)

// judgeVerdicts are the verdicts an author may declare for a non-main
// solution. They match the judge's own vocabulary so the build report can be
// compared directly against a real judging outcome.
var judgeVerdicts = map[string]bool{
	"Accepted": true, "Wrong Answer": true, "Time Limit Exceeded": true,
	"Memory Limit Exceeded": true, "Runtime Error": true,
	"Presentation Error": true, "Any Rejection": true,
}

// PackageRepository is the persistence boundary for editable package content.
type PackageRepository interface {
	Statements(ctx context.Context, problemID string) ([]Statement, error)
	SaveStatement(ctx context.Context, statement Statement) (*Statement, error)
	DeleteStatement(ctx context.Context, problemID, language string) error
	Files(ctx context.Context, problemID string, includeSource bool) ([]File, error)
	File(ctx context.Context, problemID string, id int64) (*File, error)
	SaveFile(ctx context.Context, file File) (*File, error)
	DeleteFile(ctx context.Context, problemID string, id int64) error
	Tests(ctx context.Context, problemID string, includeInput bool) ([]Test, error)
	CreateTest(ctx context.Context, test Test) (*Test, error)
	UpdateTest(ctx context.Context, test Test) (*Test, error)
	DeleteTest(ctx context.Context, problemID string, id int64) error
	ReorderTest(ctx context.Context, problemID string, id int64, target int) error
	Snapshot(ctx context.Context, problemID string) (*Package, error)
	Meta(ctx context.Context, problemID string) (*PackageMeta, error)
}

// BuildRepository is the persistence boundary for the build queue.
type BuildRepository interface {
	Enqueue(ctx context.Context, problemID, createdBy string) (*Build, error)
	Get(ctx context.Context, problemID, buildID string) (*Build, error)
	Latest(ctx context.Context, problemID string) (*Build, error)
	LatestSuccessful(ctx context.Context, problemID string) (*Build, error)
	List(ctx context.Context, problemID string, limit int) ([]Build, error)
	Cancel(ctx context.Context, problemID, buildID string) error
	Claim(ctx context.Context, workerID string, leaseTTL time.Duration) (*Build, *Package, error)
	Progress(ctx context.Context, progress Progress, leaseTTL time.Duration) error
	ResolvePackageTarget(ctx context.Context, buildID, workerID, leaseToken string) (string, error)
	RecordPackage(ctx context.Context, buildID, problemID, workerID, leaseToken string, upload PackageUpload) error
	Complete(ctx context.Context, result BuildResult, checker string) error
}

// Publisher materializes a build artifact into the shared testdata volume.
type Publisher interface {
	Publish(problemID string, archive []byte) (*PackageUpload, error)
	Remove(storagePath string) error
}

// Service holds the authoring rules: what a valid package looks like, when it
// may be built, and how a worker's fenced reports are accepted.
type Service struct {
	packages   PackageRepository
	builds     BuildRepository
	publisher  Publisher
	dispatcher *Dispatcher
	leaseTTL   time.Duration
	maxWait    time.Duration
}

func NewService(
	packages PackageRepository, builds BuildRepository, publisher Publisher,
	dispatcher *Dispatcher, leaseTTL, maxWait time.Duration,
) (*Service, error) {
	if packages == nil || builds == nil || publisher == nil || dispatcher == nil {
		return nil, errors.New("authoring dependencies must not be nil")
	}
	if leaseTTL <= 0 || maxWait <= 0 || leaseTTL <= maxWait {
		return nil, errors.New("build lease TTL must be greater than the positive long-poll timeout")
	}
	return &Service{
		packages: packages, builds: builds, publisher: publisher,
		dispatcher: dispatcher, leaseTTL: leaseTTL, maxWait: maxWait,
	}, nil
}

// ---------- statements ----------

func (s *Service) Statements(ctx context.Context, problemID string) ([]Statement, error) {
	return s.packages.Statements(ctx, problemID)
}

func (s *Service) SaveStatement(ctx context.Context, statement Statement) (*Statement, error) {
	statement.Language = strings.ToLower(strings.TrimSpace(statement.Language))
	if statement.Language == "" {
		statement.Language = "zh"
	}
	if !languagePattern.MatchString(statement.Language) {
		return nil, invalid("statement language must be a BCP-47 style tag such as zh or en")
	}
	statement.Name = strings.TrimSpace(statement.Name)
	if len(statement.Name) > 200 {
		return nil, invalid("statement name must be at most 200 characters")
	}
	for label, section := range map[string]string{
		"legend": statement.Legend, "input format": statement.InputFormat,
		"output format": statement.OutputFormat, "notes": statement.Notes,
		"tutorial": statement.Tutorial, "scoring": statement.Scoring,
	} {
		if len(section) > 200_000 {
			return nil, invalid("statement " + label + " is too long")
		}
	}
	return s.packages.SaveStatement(ctx, statement)
}

func (s *Service) DeleteStatement(ctx context.Context, problemID, language string) error {
	return s.packages.DeleteStatement(ctx, problemID, strings.ToLower(strings.TrimSpace(language)))
}

// PreviewStatement renders the public Markdown for a statement without saving
// it, using the samples produced by the last successful build.
func (s *Service) PreviewStatement(_ context.Context, statement Statement, samples []Sample) string {
	return RenderStatement(statement, samples)
}

// PublishedSamples returns the examples the last successful build produced.
// A package that has never built successfully simply has no samples yet.
func (s *Service) PublishedSamples(ctx context.Context, problemID string) ([]Sample, error) {
	build, err := s.builds.LatestSuccessful(ctx, problemID)
	if err != nil {
		return nil, err
	}
	if build == nil {
		return nil, nil
	}
	return SamplesFromOutcomes(build.Tests), nil
}

// ---------- files ----------

func (s *Service) Files(ctx context.Context, problemID string) ([]File, error) {
	return s.packages.Files(ctx, problemID, false)
}

func (s *Service) File(ctx context.Context, problemID string, id int64) (*File, error) {
	return s.packages.File(ctx, problemID, id)
}

func (s *Service) SaveFile(ctx context.Context, file File) (*File, error) {
	file.Kind = strings.TrimSpace(file.Kind)
	file.Name = strings.TrimSpace(file.Name)
	file.Language = strings.ToLower(strings.TrimSpace(file.Language))
	file.ExpectedVerdict = strings.TrimSpace(file.ExpectedVerdict)

	if !fileNamePattern.MatchString(file.Name) {
		return nil, invalid("file name must be 1-64 characters of letters, digits, '_', '.' or '-'")
	}
	if strings.TrimSpace(file.SourceCode) == "" {
		return nil, invalid("source code is required")
	}
	if len(file.SourceCode) > 1_000_000 {
		return nil, invalid("source code must be at most 1 MB")
	}

	switch file.Kind {
	case KindChecker, KindValidator, KindInteractor:
		if !testlibLanguages[file.Language] {
			return nil, invalid(file.Kind + " must be written in C++ because it links against testlib")
		}
		// Exactly one of each may be active, and a package with a single
		// checker is almost always meant to be the active one.
		file.IsActive = true
		file.ExpectedVerdict = ""
	case KindGenerator:
		if !generatorLanguage[file.Language] {
			return nil, invalid("generator language must be cpp or python")
		}
		file.IsActive = false
		file.ExpectedVerdict = ""
	case KindSolution:
		if !solutionLanguages[file.Language] {
			return nil, invalid("solution language must be c, cpp or python")
		}
		if file.IsActive {
			file.ExpectedVerdict = "Accepted"
		} else if file.ExpectedVerdict != "" && !judgeVerdicts[file.ExpectedVerdict] {
			return nil, invalid("unsupported expected verdict: " + file.ExpectedVerdict)
		}
	default:
		return nil, invalid("unsupported file kind: " + file.Kind)
	}
	return s.packages.SaveFile(ctx, file)
}

func (s *Service) DeleteFile(ctx context.Context, problemID string, id int64) error {
	return s.packages.DeleteFile(ctx, problemID, id)
}

// ---------- tests ----------

func (s *Service) Tests(ctx context.Context, problemID string) ([]Test, error) {
	return s.packages.Tests(ctx, problemID, false)
}

func (s *Service) CreateTest(ctx context.Context, test Test) (*Test, error) {
	prepared, err := prepareTest(test)
	if err != nil {
		return nil, err
	}
	return s.packages.CreateTest(ctx, prepared)
}

func (s *Service) UpdateTest(ctx context.Context, test Test) (*Test, error) {
	prepared, err := prepareTest(test)
	if err != nil {
		return nil, err
	}
	return s.packages.UpdateTest(ctx, prepared)
}

func (s *Service) DeleteTest(ctx context.Context, problemID string, id int64) error {
	return s.packages.DeleteTest(ctx, problemID, id)
}

func (s *Service) ReorderTest(ctx context.Context, problemID string, id int64, target int) error {
	return s.packages.ReorderTest(ctx, problemID, id, target)
}

func prepareTest(test Test) (Test, error) {
	test.Source = strings.TrimSpace(test.Source)
	test.Group = strings.TrimSpace(test.Group)
	test.Description = strings.TrimSpace(test.Description)
	test.GenerateCmd = strings.TrimSpace(test.GenerateCmd)

	switch test.Source {
	case TestManual:
		if strings.TrimSpace(test.InputData) == "" {
			return Test{}, invalid("manual tests need input data")
		}
		if len(test.InputData) > 4<<20 {
			return Test{}, invalid("manual test input must be at most 4 MB; use a generator instead")
		}
		test.GenerateCmd = ""
	case TestGenerator:
		if _, _, err := ParseGenerateCommand(test.GenerateCmd); err != nil {
			return Test{}, err
		}
		test.InputData = ""
	default:
		return Test{}, invalid("test source must be manual or generator")
	}
	if test.Group != "" && !fileNamePattern.MatchString(test.Group) {
		return Test{}, invalid("test group must be 1-64 characters of letters, digits, '_', '.' or '-'")
	}
	if test.Points < 0 || test.Points > 1000 {
		return Test{}, invalid("test points must be between 0 and 1000")
	}
	if len(test.Description) > 500 {
		return Test{}, invalid("test description must be at most 500 characters")
	}
	return test, nil
}

// ParseGenerateCommand splits a Polygon-style generator line into the
// generator name and its arguments. The worker executes argv directly, never
// through a shell, and the token rules here keep it that way.
func ParseGenerateCommand(command string) (string, []string, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, invalid("generator command is required")
	}
	if len(fields) > 33 {
		return "", nil, invalid("generator command accepts at most 32 arguments")
	}
	name := fields[0]
	if !fileNamePattern.MatchString(name) {
		return "", nil, invalid("generator name must be 1-64 characters of letters, digits, '_', '.' or '-'")
	}
	arguments := fields[1:]
	for _, argument := range arguments {
		if !generatorArgPattern.MatchString(argument) {
			return "", nil, invalid("generator argument contains unsupported characters: " + argument)
		}
	}
	return name, arguments, nil
}

// ---------- builds ----------

// Workspace is the authoring summary the editor loads in one request: what the
// package contains, whether the published testdata still matches it, and what
// currently blocks a build.
type Workspace struct {
	Meta        PackageMeta
	Statements  []Statement
	Files       []File
	Tests       []Test
	LatestBuild *Build
	Issues      []string
}

// Workspace assembles the authoring view. Sources and test inputs are omitted;
// the editor fetches those one at a time.
func (s *Service) Workspace(ctx context.Context, problemID string) (*Workspace, error) {
	meta, err := s.packages.Meta(ctx, problemID)
	if err != nil {
		return nil, err
	}
	statements, err := s.packages.Statements(ctx, problemID)
	if err != nil {
		return nil, err
	}
	files, err := s.packages.Files(ctx, problemID, false)
	if err != nil {
		return nil, err
	}
	tests, err := s.packages.Tests(ctx, problemID, false)
	if err != nil {
		return nil, err
	}
	latest, err := s.builds.Latest(ctx, problemID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.packages.Snapshot(ctx, problemID)
	if err != nil {
		return nil, err
	}
	return &Workspace{
		Meta: *meta, Statements: statements, Files: files, Tests: tests,
		LatestBuild: latest, Issues: Validate(snapshot),
	}, nil
}

// Validate reports the reasons a package cannot be built yet. An empty result
// means Build would be accepted.
func Validate(pkg *Package) []string {
	problems := make([]string, 0, 4)
	if len(pkg.Tests) == 0 {
		problems = append(problems, "至少需要一个测试点")
	}
	if pkg.MainSolution() == nil {
		problems = append(problems, "需要一个标程(设为主解的 solution)")
	}
	generators := make(map[string]bool, len(pkg.Generators))
	for _, generator := range pkg.Generators {
		generators[generator.Name] = true
	}
	for _, test := range pkg.Tests {
		if test.Source != TestGenerator {
			continue
		}
		name, _, err := ParseGenerateCommand(test.GenerateCmd)
		if err != nil {
			problems = append(problems, fmt.Sprintf("测试点 %d 的生成命令无效: %s", test.Index, err.Error()))
			continue
		}
		if !generators[name] {
			problems = append(problems, fmt.Sprintf("测试点 %d 引用了不存在的生成器 %q", test.Index, name))
		}
	}
	if pkg.JudgeType == "interactive" && pkg.Interactor == nil {
		problems = append(problems, "交互题需要一个 interactor")
	}
	return problems
}

// Build validates the package and queues one build. A package that is already
// building returns the running build with ErrBuildRunning so the caller can
// show it instead of creating a duplicate.
func (s *Service) Build(ctx context.Context, problemID, requestedBy string) (*Build, error) {
	pkg, err := s.packages.Snapshot(ctx, problemID)
	if err != nil {
		return nil, err
	}
	if issues := Validate(pkg); len(issues) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotBuildable, strings.Join(issues, "; "))
	}
	build, err := s.builds.Enqueue(ctx, problemID, requestedBy)
	if err != nil {
		return build, err
	}
	s.dispatcher.Notify()
	return build, nil
}

func (s *Service) BuildStatus(ctx context.Context, problemID, buildID string) (*Build, error) {
	return s.builds.Get(ctx, problemID, buildID)
}

func (s *Service) LatestBuild(ctx context.Context, problemID string) (*Build, error) {
	return s.builds.Latest(ctx, problemID)
}

func (s *Service) Builds(ctx context.Context, problemID string, limit int) ([]Build, error) {
	return s.builds.List(ctx, problemID, limit)
}

func (s *Service) CancelBuild(ctx context.Context, problemID, buildID string) error {
	return s.builds.Cancel(ctx, problemID, buildID)
}

// Claim long-polls for one leased build job, mirroring the judge protocol so
// workers can run both loops with the same failure handling.
func (s *Service) Claim(ctx context.Context, workerID string, wait time.Duration) (*Build, *Package, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || len(workerID) > 128 {
		return nil, nil, invalid("worker ID must contain 1-128 characters")
	}
	if wait <= 0 || wait > s.maxWait {
		wait = s.maxWait
	}
	deadline := time.Now().Add(wait)
	for {
		build, pkg, err := s.builds.Claim(ctx, workerID, s.leaseTTL)
		if err != nil {
			return nil, nil, err
		}
		if build != nil {
			s.dispatcher.Notify()
			return build, pkg, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil, nil
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, nil, ctx.Err()
		case <-s.dispatcher.wake:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

func (s *Service) Progress(ctx context.Context, progress Progress) error {
	if progress.BuildID == "" || progress.WorkerID == "" || progress.LeaseToken == "" {
		return ErrStaleLease
	}
	if progress.Done < 0 {
		progress.Done = 0
	}
	if progress.Total < 0 {
		progress.Total = 0
	}
	if len(progress.Log) > 64<<10 {
		progress.Log = progress.Log[:64<<10]
	}
	return s.builds.Progress(ctx, progress, s.leaseTTL)
}

// UploadPackage materializes an artifact for a leased build. Publication is
// deferred to Complete so a worker that dies mid-report can never leave the
// problem pointing at data whose build never finished.
func (s *Service) UploadPackage(
	ctx context.Context, buildID, requestedProblemID, workerID, leaseToken string, archive []byte,
) (*PackageUpload, error) {
	if buildID == "" || workerID == "" || leaseToken == "" {
		return nil, ErrStaleLease
	}
	problemID, err := s.builds.ResolvePackageTarget(ctx, buildID, workerID, leaseToken)
	if err != nil {
		return nil, err
	}
	if requestedProblemID != problemID {
		return nil, ErrPackageTarget
	}
	upload, err := s.publisher.Publish(problemID, archive)
	if err != nil {
		return nil, err
	}
	if err := s.builds.RecordPackage(ctx, buildID, problemID, workerID, leaseToken, *upload); err != nil {
		if upload.created {
			if cleanupErr := s.publisher.Remove(upload.StoragePath); cleanupErr != nil {
				return nil, errors.Join(err, fmt.Errorf("remove rejected build package: %w", cleanupErr))
			}
		}
		return nil, err
	}
	return upload, nil
}

// Complete stores a fenced build result and, on success, publishes the
// recorded artifact as the problem's testdata.
func (s *Service) Complete(ctx context.Context, result BuildResult, checker string) error {
	if result.BuildID == "" || result.WorkerID == "" || result.LeaseToken == "" {
		return ErrStaleLease
	}
	if checker != "diff" && checker != "testlib" && checker != "interactive" {
		checker = "diff"
	}
	if len(result.Tests) > 10000 {
		return invalid("build reported too many tests")
	}
	for i := range result.Tests {
		if result.Tests[i].Index <= 0 {
			return invalid("build reported an invalid test index")
		}
	}
	return s.builds.Complete(ctx, result, checker)
}
