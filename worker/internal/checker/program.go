package checker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// Program is prepared by the compiler, never directly from a client command.
// It supports interpreted validators and their runtime companion files too.
type Program struct {
	Command []string
	Files   map[string]run.InputFile
}

func (runner *Runner) CheckProgram(ctx context.Context, program Program, protocol string, args []string, inputPath, outputPath, answerPath string) (Verdict, error) {
	if protocol != "testlib" && protocol != "kattis" {
		return Verdict{}, fmt.Errorf("unknown output validator protocol %q", protocol)
	}
	if len(program.Command) == 0 {
		return Verdict{}, fmt.Errorf("validator command is empty")
	}
	env, err := runner.sandbox.Create(ctx, run.EnvironmentPolicy{MemoryKB: runner.limits.MemoryKB, Processes: runner.limits.Processes})
	if err != nil {
		return Verdict{}, err
	}
	defer env.Close()
	const inputName = "__vertex_input"
	const outputName = "__vertex_output"
	const answerName = "__vertex_answer"
	inputs := map[string]run.InputFile{}
	for name, file := range program.Files {
		if name == inputName || name == outputName || name == answerName || name == "judgemessage.txt" || name == "teammessage.txt" || name == "score.txt" {
			return Verdict{}, fmt.Errorf("validator file uses reserved name %s", name)
		}
		inputs[name] = file
	}
	inputs[inputName] = run.InputFile{Path: inputPath}
	inputs[outputName] = run.InputFile{Path: outputPath}
	inputs[answerName] = run.InputFile{Path: answerPath}
	if err := env.PutFiles(ctx, inputs); err != nil {
		return Verdict{}, err
	}
	command := append([]string{}, program.Command...)
	stdin := ""
	if protocol == "kattis" {
		command = append(command, inputName, answerName, "./")
		stdin = outputName
	} else {
		command = append(command, inputName, outputName, answerName)
	}
	command = append(command, args...)
	result, err := env.Run(ctx, run.Execution{Command: command, StdinFile: stdin, Limits: run.Limits{CPUTime: runner.limits.CPUTime, WallTime: runner.limits.CPUTime * 2, MemoryKB: runner.limits.MemoryKB, Processes: runner.limits.Processes, OutputBytes: MaxOutputBytes}})
	if err != nil {
		return Verdict{}, err
	}
	if result.Meta == nil {
		return Verdict{}, fmt.Errorf("validator execution returned no metadata")
	}
	decision := validatorDecision(*result.Meta, protocol)
	if protocol == "testlib" {
		if comment := readCheckerComment(result.Stderr, result.Stdout); comment != "" {
			if decision.Verdict == verdict.SE {
				decision.Message += ": " + comment
			} else {
				decision.Message = comment
			}
		}
		return decision, nil
	}
	// Kattis sends team output on stdin and writes separate public/jury feedback.
	// The existing sandbox workspace is a valid feedback directory (./). Exports
	// remain bounded and reject symlinks just like every other sandbox artifact.
	directory, err := os.MkdirTemp("", "vertex-validator-feedback-")
	if err != nil {
		return Verdict{}, err
	}
	defer os.RemoveAll(directory)
	readFeedback := func(name string) string {
		target := filepath.Join(directory, name)
		if err := env.ExportFile(ctx, name, target, 64<<10); err != nil {
			return ""
		}
		data, err := os.ReadFile(target)
		if err != nil {
			return ""
		}
		text := strings.TrimSpace(string(data))
		if len(text) > maxCheckerMessageBytes {
			text = text[:maxCheckerMessageBytes] + "…"
		}
		return text
	}
	decision.JudgeMessage = readFeedback("judgemessage.txt")
	if decision.JudgeMessage == "" {
		decision.JudgeMessage = readCheckerComment(result.Stderr, result.Stdout)
	}
	if team := readFeedback("teammessage.txt"); team != "" && decision.Verdict != verdict.SE {
		decision.Message = team
	}
	if err := ctx.Err(); err != nil {
		return Verdict{}, err
	}
	return decision, nil
}

func validatorDecision(meta run.Meta, protocol string) Verdict {
	if limit := verdict.FromSandboxMeta(&meta); limit != "" && limit != verdict.RE {
		return Verdict{Verdict: verdict.SE, Message: "output validator exceeded its execution limits"}
	}
	if meta.ExitSignal != 0 || meta.Killed || meta.CgOOMKilled || meta.OutputLimit || (meta.TerminationReason != "" && meta.TerminationReason != run.TerminationExited) || (meta.Status != "" && meta.Status != "RE") {
		return Verdict{Verdict: verdict.SE, Message: "output validator did not complete within its execution limits"}
	}
	if protocol == "kattis" {
		switch meta.ExitCode {
		case 42:
			return Verdict{Verdict: verdict.AC, Message: "ok"}
		case 43:
			return Verdict{Verdict: verdict.WA, Message: "output rejected by validator"}
		default:
			return Verdict{Verdict: verdict.SE, Message: fmt.Sprintf("output validator exited with unexpected code %d", meta.ExitCode)}
		}
	}
	if protocol == "testlib" {
		switch meta.ExitCode {
		case exitOK:
			return Verdict{Verdict: verdict.AC, Message: "ok"}
		case exitWrongAnswer, exitPresentation, exitDirt, exitUnexpectedEOF:
			return Verdict{Verdict: verdict.WA, Message: "output rejected by validator"}
		case exitPoints:
			return Verdict{Verdict: verdict.SE, Message: "checker returned a partial score without a scoring protocol"}
		default:
			return Verdict{Verdict: verdict.SE, Message: fmt.Sprintf("checker exited with unexpected code %d", meta.ExitCode)}
		}
	}
	return Verdict{Verdict: verdict.SE, Message: "unknown output validator protocol"}
}

func ValidatorAccepted(meta run.Meta, protocol string) (bool, error) {
	code := 0
	switch protocol {
	case "kattis":
		code = 42
	case "testlib", "stdio":
	default:
		return false, fmt.Errorf("unknown input validator protocol %q", protocol)
	}
	return ValidatorExitedCleanly(meta) && meta.ExitCode == code, nil
}

// A deliberate nonzero rejection is a clean exit. Signals, resource limits and
// sandbox setup failures must never count as successful rejection self-tests.
func ValidatorExitedCleanly(meta run.Meta) bool {
	clean := meta.ExitSignal == 0 && !meta.Killed && !meta.CgOOMKilled && !meta.OutputLimit && !meta.WorkspaceLimit && (meta.TerminationReason == "" || meta.TerminationReason == run.TerminationExited) && (meta.Status == "" || meta.Status == "RE")
	if limit := verdict.FromSandboxMeta(&meta); limit != "" && limit != verdict.RE {
		clean = false
	}
	return clean
}
