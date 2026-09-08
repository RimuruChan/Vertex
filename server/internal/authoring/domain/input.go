package domain

import (
	"fmt"
	"regexp"
	"strings"
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

func NormalizeStatement(statement Statement) (Statement, error) {
	statement.Language = strings.ToLower(strings.TrimSpace(statement.Language))
	if statement.Language == "" {
		statement.Language = "zh"
	}
	if !languagePattern.MatchString(statement.Language) {
		return Statement{}, InvalidInput("statement language must be a BCP-47 style tag such as zh or en")
	}
	statement.Name = strings.TrimSpace(statement.Name)
	if len(statement.Name) > 200 {
		return Statement{}, InvalidInput("statement name must be at most 200 characters")
	}
	for label, section := range map[string]string{
		"legend": statement.Legend, "input format": statement.InputFormat,
		"output format": statement.OutputFormat, "notes": statement.Notes,
		"tutorial": statement.Tutorial, "scoring": statement.Scoring,
	} {
		if len(section) > 200_000 {
			return Statement{}, InvalidInput("statement " + label + " is too long")
		}
	}
	return statement, nil
}

func NormalizeFile(file File) (File, error) {
	file.Kind = strings.TrimSpace(file.Kind)
	file.Name = strings.TrimSpace(file.Name)
	file.Language = strings.ToLower(strings.TrimSpace(file.Language))
	file.ExpectedVerdict = strings.TrimSpace(file.ExpectedVerdict)

	if !fileNamePattern.MatchString(file.Name) {
		return File{}, InvalidInput("file name must be 1-64 characters of letters, digits, '_', '.' or '-'")
	}
	if strings.TrimSpace(file.SourceCode) == "" {
		return File{}, InvalidInput("source code is required")
	}
	if len(file.SourceCode) > 1_000_000 {
		return File{}, InvalidInput("source code must be at most 1 MB")
	}

	switch file.Kind {
	case KindChecker, KindValidator, KindInteractor:
		if !testlibLanguages[file.Language] {
			return File{}, InvalidInput(file.Kind + " must be written in C++ because it links against testlib")
		}
		// Exactly one of each may be active, and a package with a single
		// checker is almost always meant to be the active one.
		file.IsActive = true
		file.ExpectedVerdict = ""
	case KindGenerator:
		if !generatorLanguage[file.Language] {
			return File{}, InvalidInput("generator language must be cpp or python")
		}
		file.IsActive = false
		file.ExpectedVerdict = ""
	case KindSolution:
		if !solutionLanguages[file.Language] {
			return File{}, InvalidInput("solution language must be c, cpp or python")
		}
		if file.IsActive {
			file.ExpectedVerdict = "Accepted"
		} else if file.ExpectedVerdict != "" && !judgeVerdicts[file.ExpectedVerdict] {
			return File{}, InvalidInput("unsupported expected verdict: " + file.ExpectedVerdict)
		}
	default:
		return File{}, InvalidInput("unsupported file kind: " + file.Kind)
	}
	return file, nil
}

func NormalizeTest(test Test) (Test, error) {
	test.Source = strings.TrimSpace(test.Source)
	test.Group = strings.TrimSpace(test.Group)
	test.Description = strings.TrimSpace(test.Description)
	test.GenerateCmd = strings.TrimSpace(test.GenerateCmd)

	switch test.Source {
	case TestManual:
		if strings.TrimSpace(test.InputData) == "" {
			return Test{}, InvalidInput("manual tests need input data")
		}
		if len(test.InputData) > 4<<20 {
			return Test{}, InvalidInput("manual test input must be at most 4 MB; use a generator instead")
		}
		test.GenerateCmd = ""
	case TestGenerator:
		if _, _, err := ParseGenerateCommand(test.GenerateCmd); err != nil {
			return Test{}, err
		}
		test.InputData = ""
	default:
		return Test{}, InvalidInput("test source must be manual or generator")
	}
	if test.Group != "" && !fileNamePattern.MatchString(test.Group) {
		return Test{}, InvalidInput("test group must be 1-64 characters of letters, digits, '_', '.' or '-'")
	}
	if test.Points < 0 || test.Points > 1000 {
		return Test{}, InvalidInput("test points must be between 0 and 1000")
	}
	if len(test.Description) > 500 {
		return Test{}, InvalidInput("test description must be at most 500 characters")
	}
	return test, nil
}

// ParseGenerateCommand splits a Polygon-style generator line into the
// generator name and its arguments. The worker executes argv directly, never
// through a shell, and the token rules here keep it that way.
func ParseGenerateCommand(command string) (string, []string, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, InvalidInput("generator command is required")
	}
	if len(fields) > 33 {
		return "", nil, InvalidInput("generator command accepts at most 32 arguments")
	}
	name := fields[0]
	if !fileNamePattern.MatchString(name) {
		return "", nil, InvalidInput("generator name must be 1-64 characters of letters, digits, '_', '.' or '-'")
	}
	arguments := fields[1:]
	for _, argument := range arguments {
		if !generatorArgPattern.MatchString(argument) {
			return "", nil, InvalidInput("generator argument contains unsupported characters: " + argument)
		}
	}
	return name, arguments, nil
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

func NormalizePublication(input PublishInput) (PublishInput, error) {
	if input.Revision < 0 || input.ArtifactVersion <= 0 {
		return PublishInput{}, InvalidInput("invalid working revision or candidate version")
	}
	input.Language = strings.TrimSpace(input.Language)
	if input.Language != "" && !languagePattern.MatchString(input.Language) {
		return PublishInput{}, InvalidInput("invalid statement language")
	}
	return input, nil
}
