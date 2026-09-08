package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
)

// program is a compiled package source ready to run in the sandbox. Path is a
// host-side artifact: a binary for compiled languages, the source file itself
// for interpreted ones.
type program struct {
	Name   string
	Config compile.LangConfig
	Path   string
}

// boxName is the workspace file name the artifact is copied to. Interpreted
// languages keep their source extension because the interpreter is invoked
// with that name.
func (p program) boxName() string {
	if p.Config.CompileCmd == nil {
		return p.Config.SourceExt
	}
	return "prog"
}

// command renders the run command with the box-relative artifact name and
// appends the caller's arguments.
func (p program) command(arguments ...string) []string {
	rendered := make([]string, 0, len(p.Config.RunCmd)+len(arguments))
	for _, argument := range p.Config.RunCmd {
		rendered = append(rendered, strings.ReplaceAll(argument, "{exe}", p.boxName()))
	}
	return append(rendered, arguments...)
}

// compileSource builds one package source. testlib is linked into the
// checker, validator, interactor and every C++ generator, which is exactly the
// set Polygon compiles against testlib too.
func (b *Builder) compileSource(ctx context.Context, file SourceFile, withTestlib bool) (*program, error) {
	config, ok := compile.Supported[file.Language]
	if !ok {
		return nil, fmt.Errorf("%s: unsupported language %q", file.Name, file.Language)
	}
	digest := sha256.Sum256([]byte(file.SourceCode))
	sourceHash := hex.EncodeToString(digest[:])

	extension := compile.Extension{}
	if withTestlib && config.CompileCmd != nil {
		extension = checker.TestlibExtension(b.testlibPath, b.testlibDigest)
	}
	path, result := b.compiler.CompileExt(ctx, file.Language, []byte(file.SourceCode), sourceHash, extension)
	if !result.OK {
		return nil, fmt.Errorf("%s: %s", file.Name, result.Error)
	}
	return &program{Name: file.Name, Config: config, Path: path}, nil
}

// execution describes one sandboxed run of a package program.
type execution struct {
	program   *program
	arguments []string
	// stdinPath is a host-side file copied into the workspace as the standard
	// input of the run.
	stdinPath string
	// extraFiles are additional host files to copy in, keyed by box name.
	extraFiles map[string]string
	timeLimit  time.Duration
	memoryKB   int
	outputCap  int64
	stdoutPath string
}

// runProgram owns a fresh environment. Requested output is exported to the
// caller's staging directory before the environment is closed.
func (b *Builder) runProgram(ctx context.Context, request execution) (*run.Result, error) {

	boxName := request.program.boxName()
	inputs := map[string]run.InputFile{boxName: {Path: request.program.Path, Executable: request.program.Config.CompileCmd != nil}}
	for name, path := range request.extraFiles {
		inputs[name] = run.InputFile{Path: path}
	}
	stdinName := ""
	if request.stdinPath != "" {
		stdinName = "stdin.txt"
		inputs[stdinName] = run.InputFile{Path: request.stdinPath}
	}

	config := request.program.Config
	cpuLimit := time.Duration(float64(request.timeLimit) * config.TimeFactor)
	memoryKB := int(float64(request.memoryKB)*config.MemFactor) + config.MemAddKB
	env, err := b.sandbox.Create(ctx, run.EnvironmentPolicy{MemoryKB: memoryKB, Processes: config.ProcAllow})
	if err != nil {
		return nil, err
	}
	defer env.Close()
	if err := env.PutFiles(ctx, inputs); err != nil {
		return nil, err
	}
	return env.Run(ctx, run.Execution{
		Command:    request.program.command(request.arguments...),
		StdoutPath: request.stdoutPath,
		StdinFile:  stdinName,
		Limits: run.Limits{
			CPUTime:     cpuLimit,
			WallTime:    cpuLimit * 2,
			MemoryKB:    memoryKB,
			Processes:   config.ProcAllow,
			OutputBytes: request.outputCap,
		},
	})
}
