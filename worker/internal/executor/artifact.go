package executor

import (
	"context"
	"fmt"

	"github.com/RimuruChan/Vertex/worker/internal/artifact"
	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

type ArtifactGrader struct {
	policy    checker.Policy
	program   *compile.Artifact
	protocol  string
	arguments []string
	runner    *checker.Runner
}

func (grader *ArtifactGrader) Grade(ctx context.Context, input, output, answer string) (string, string) {
	var decision checker.Verdict
	var err error
	if grader.program == nil {
		decision, err = checker.CheckOutput(output, answer, grader.policy)
	} else {
		decision, err = grader.runner.CheckProgram(ctx, checker.Program{Command: grader.program.Command, Files: grader.program.Files}, grader.protocol, grader.arguments, input, output, answer)
	}
	if err != nil {
		return verdict.SE, "output validation failed"
	}
	// Privileged jury feedback never becomes contestant feedback.
	return decision.Verdict, decision.Message
}

func (executor *Executor) ArtifactGrader(ctx context.Context, directory, expectedHash string, compiler *compile.Compiler) (Grader, []Case, error) {
	if err := artifact.VerifyDirectory(directory, expectedHash); err != nil {
		return nil, nil, err
	}
	manifest, err := artifact.Read(directory)
	if err != nil {
		return nil, nil, err
	}
	policy := manifest.Snapshot.Metadata.Comparison
	grader := &ArtifactGrader{policy: policy, runner: executor.checkerRunner}
	if policy.Kind == "tokens" || policy.Kind == "exact" {
		if err := policy.Validate(); err != nil {
			return nil, nil, err
		}
	} else if policy.Kind == "testlib" || policy.Kind == "kattis" {
		var selected *artifact.FrozenProgram
		for index := range manifest.Snapshot.Programs {
			program := &manifest.Snapshot.Programs[index]
			if program.ID == manifest.Snapshot.Metadata.OutputValidator {
				selected = program
				break
			}
		}
		if selected == nil || selected.Definition.Role != "output-validator" || selected.Definition.Protocol != policy.Kind || grader.runner == nil {
			return nil, nil, fmt.Errorf("packaged output validator is unavailable")
		}
		files := map[string]string{}
		for _, source := range selected.Files {
			file, err := artifact.Verify(directory, "programs/"+selected.ID+"/"+source.Path, source.Blob)
			if err != nil {
				return nil, nil, err
			}
			files[source.Path] = file
		}
		extension := compile.Extension{}
		if selected.Definition.Language == "cpp" && files["testlib.h"] == "" {
			for _, dependency := range manifest.Dependencies {
				if dependency.ID == "testlib" && dependency.Path == "testlib.h" {
					file, err := artifact.Verify(directory, "dependencies/testlib.h", dependency.Blob)
					if err != nil {
						return nil, nil, err
					}
					extension = checker.TestlibExtension(file, dependency.Blob.SHA256)
				}
			}
			if extension.Files == nil {
				return nil, nil, fmt.Errorf("pinned testlib dependency is missing")
			}
		}
		program, result := compiler.CompileFiles(ctx, selected.Definition.Language, selected.EntryPoint, files, extension)
		if !result.OK {
			return nil, nil, fmt.Errorf("packaged checker compilation failed: %s", result.Error)
		}
		grader.program = program
		grader.protocol = selected.Definition.Protocol
		grader.arguments = selected.Definition.Arguments
	} else {
		return nil, nil, fmt.Errorf("unsupported artifact comparison protocol")
	}
	cases := make([]Case, 0, len(manifest.Tests))
	for index, test := range manifest.Tests {
		definition := manifest.Snapshot.Tests[index]
		if definition.ID != test.ID {
			return nil, nil, fmt.Errorf("artifact test order mismatch")
		}
		input, err := artifact.Verify(directory, fmt.Sprintf("%d.in", index+1), test.Input)
		if err != nil {
			return nil, nil, err
		}
		answer, err := artifact.Verify(directory, fmt.Sprintf("%d.out", index+1), test.Answer)
		if err != nil {
			return nil, nil, err
		}
		timeLimit, memory := definition.Definition.TimeLimitMs, definition.Definition.MemoryLimitKB
		if timeLimit == 0 {
			timeLimit = manifest.Snapshot.Metadata.TimeLimitMs
		}
		if memory == 0 {
			memory = manifest.Snapshot.Metadata.MemoryLimitKB
		}
		if timeLimit <= 0 || timeLimit > 3600000 || memory <= 0 || memory > 1<<30 {
			return nil, nil, fmt.Errorf("invalid artifact resource limits")
		}
		cases = append(cases, Case{Index: index + 1, InputPath: input, ExpectedPath: answer, TimeLimitMs: timeLimit, MemLimitKB: memory, ExactLimits: manifest.Snapshot.Metadata.ResourceMode == "exact"})
	}
	return grader, cases, nil
}
