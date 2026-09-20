package compile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

const bundleExecutable = "__vertex_program"
const maxBundleBytes = int64(32 << 20)

type Artifact struct {
	EntryPoint string
	Command    []string
	Files      map[string]run.InputFile
}

// CompileFiles preserves the declared program layout, including headers and
// runtime companions. Every compile runs inside a fresh native environment.
func (c *Compiler) CompileFiles(ctx context.Context, language, entryPoint string, files map[string]string, extension Extension) (*Artifact, *Result) {
	config, ok := Supported[language]
	if !ok {
		return nil, &Result{Error: "unsupported language: " + language}
	}
	if _, ok := files[entryPoint]; !ok {
		return nil, &Result{Error: "program entry point is missing"}
	}
	inputs := map[string]string{}
	for name, file := range files {
		inputs[name] = file
	}
	for name, file := range extension.Files {
		if _, exists := inputs[name]; exists {
			return nil, &Result{Error: "duplicate compiler input: " + name}
		}
		inputs[name] = file
	}
	digest, err := bundleDigest(ctx, entryPoint, inputs)
	if err != nil {
		return nil, &Result{Error: err.Error()}
	}
	artifact := &Artifact{EntryPoint: entryPoint, Files: map[string]run.InputFile{}}
	for name, file := range files {
		artifact.Files[name] = run.InputFile{Path: file}
	}
	if config.CompileCmd == nil {
		for _, part := range config.RunCmd {
			artifact.Command = append(artifact.Command, strings.ReplaceAll(part, "{exe}", "./"+entryPoint))
		}
		return artifact, &Result{OK: true}
	}
	version, err := resolveToolchainVersion(ctx, config.ToolchainVersionCmd)
	if err != nil {
		return nil, &Result{Error: "toolchain version: " + err.Error()}
	}
	cacheFile := filepath.Join(c.CacheDir, cacheFingerprint(language, config, digest, version, extension))
	if info, err := os.Lstat(cacheFile); err == nil && info.Mode().IsRegular() {
		return compiledArtifact(artifact, cacheFile), &Result{OK: true, OutputDir: filepath.Dir(cacheFile)}
	}
	arguments, err := bundleCommand(language, config, entryPoint, inputs, extension.Args)
	if err != nil {
		return nil, &Result{Error: err.Error()}
	}
	work, err := os.MkdirTemp(c.ScratchDir, "compile-bundle-")
	if err != nil {
		return nil, &Result{Error: err.Error()}
	}
	defer os.RemoveAll(work)
	env, err := c.sandbox.Create(ctx, run.EnvironmentPolicy{MemoryKB: config.CompilerMemKB, Processes: config.ProcAllow})
	if err != nil {
		return nil, &Result{Error: "create compiler environment: " + err.Error()}
	}
	defer env.Close()
	staged := map[string]run.InputFile{}
	for name, file := range inputs {
		staged[name] = run.InputFile{Path: file}
	}
	if err := env.PutFiles(ctx, staged); err != nil {
		return nil, &Result{Error: "stage compiler inputs: " + err.Error()}
	}
	result, err := env.Run(ctx, run.Execution{Command: arguments, StdoutPath: filepath.Join(work, "stdout"), StderrPath: filepath.Join(work, "stderr"), Limits: run.Limits{CPUTime: time.Duration(config.CompilerTimeMs) * time.Millisecond, WallTime: time.Duration(config.CompilerTimeMs) * 2 * time.Millisecond, MemoryKB: config.CompilerMemKB, Processes: config.ProcAllow, OutputBytes: 8 << 20}})
	if err != nil {
		return nil, &Result{Error: "run compiler: " + err.Error()}
	}
	if result.Meta == nil {
		return nil, &Result{Error: "compiler returned no execution metadata"}
	}
	if verdict.FromSandboxMeta(result.Meta) != "" {
		message := result.Stderr
		if len(message) > 8192 {
			message = message[:8192]
		}
		return nil, &Result{Error: "compile failed: " + result.Meta.ExitDescription() + "\n" + message}
	}
	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return nil, &Result{Error: err.Error()}
	}
	if err := env.ExportFile(ctx, bundleExecutable, cacheFile, c.sandbox.Policy.WorkspaceBytes); err != nil {
		info, statErr := os.Lstat(cacheFile)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, &Result{Error: "cache compiled bundle: " + err.Error()}
		}
	}
	return compiledArtifact(artifact, cacheFile), &Result{OK: true, OutputDir: filepath.Dir(cacheFile)}
}

func compiledArtifact(artifact *Artifact, file string) *Artifact {
	artifact.EntryPoint = bundleExecutable
	artifact.Command = []string{"./" + bundleExecutable}
	artifact.Files[bundleExecutable] = run.InputFile{Path: file, Executable: true}
	return artifact
}

func bundleDigest(ctx context.Context, entryPoint string, files map[string]string) (string, error) {
	if err := run.ValidateInputPath(entryPoint); err != nil {
		return "", err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if err := run.ValidateInputPath(name); err != nil {
			return "", err
		}
		if name == bundleExecutable {
			return "", fmt.Errorf("reserved program file name")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	fmt.Fprintf(hash, "vertex-program-bundle-1\x00%s\x00", entryPoint)
	remaining := maxBundleBytes
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		file, err := os.Open(files[name])
		if err != nil {
			return "", err
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			file.Close()
			return "", fmt.Errorf("invalid program source %s", name)
		}
		fileHash := sha256.New()
		size, err := io.Copy(fileHash, io.LimitReader(file, remaining+1))
		file.Close()
		if err != nil {
			return "", err
		}
		if size > remaining {
			return "", fmt.Errorf("program sources exceed 32 MiB")
		}
		remaining -= size
		fmt.Fprintf(hash, "%d:%s:%d:%x\n", len(name), name, size, fileHash.Sum(nil))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func bundleCommand(language string, config LangConfig, entry string, files map[string]string, extra []string) ([]string, error) {
	if language != "cpp" && language != "c" {
		return nil, fmt.Errorf("unsupported compiled bundle language %s", language)
	}
	units := []string{entry}
	var rest []string
	for name := range files {
		if name == entry {
			continue
		}
		ext := strings.ToLower(path.Ext(name))
		if ext == ".c" || language == "cpp" && (ext == ".cc" || ext == ".cpp" || ext == ".cxx" || ext == ".c++") {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	units = append(units, rest...)
	var command []string
	for _, argument := range config.CompileCmd {
		if argument == "{in}" {
			dialect := "c"
			if language == "cpp" {
				dialect = "c++"
			}
			command = append(command, "-x", dialect)
			for _, name := range units {
				command = append(command, "./"+name)
			}
			command = append(command, "-x", "none")
		} else {
			command = append(command, strings.ReplaceAll(argument, "{out}", bundleExecutable))
		}
	}
	return append(command, extra...), nil
}

// ToolchainFingerprint records actual compiler/interpreter versions as well as
// language settings. The caller supplies sandbox, dependency and stage policy.
func ToolchainFingerprint(ctx context.Context, languages []string, policy string) (string, error) {
	seen := map[string]bool{}
	for _, language := range languages {
		seen[language] = true
	}
	names := make([]string, 0, len(seen))
	for language := range seen {
		names = append(names, language)
	}
	sort.Strings(names)
	hash := sha256.New()
	fmt.Fprintf(hash, "vertex-toolchain-1\x00%s\x00", policy)
	for _, language := range names {
		config, ok := Supported[language]
		if !ok {
			return "", fmt.Errorf("unsupported program language %s", language)
		}
		command := config.ToolchainVersionCmd
		if language == "python" {
			command = []string{"/usr/bin/python3", "--version"}
		}
		version, err := resolveToolchainVersion(ctx, command)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(config)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00", language, encoded, version)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
