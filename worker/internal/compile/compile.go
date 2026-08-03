package compile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// LangConfig 一种语言的编译与运行配置。
type LangConfig struct {
	Name                string
	SourceExt           string   // 源码文件名扩展
	CompileCmd          []string // 编译命令;{in} 替换为源码路径,{out} 替换为输出文件
	ToolchainVersionCmd []string // 输出真实工具链版本，用于编译缓存版本化
	RunCmd              []string // 运行命令;{exe} 替换为编译产物路径
	TimeFactor          float64  // CPU 时间倍率
	MemFactor           float64  // 内存倍率
	MemAddKB            int      // 额外内存(KB),如 JVM 基础开销
	ProcAllow           int      // 允许的进程/线程数
	CompilerTimeMs      int      // 编译自身的 CPU 时间上限
	CompilerMemKB       int      // 编译自身的内存上限
}

// Supported 判题语言注册表。
// 新增语言:在此加一条,并确保 Judge 镜像内有对应工具链。
var Supported = map[string]LangConfig{
	"c": {
		Name: "c", SourceExt: "main.c",
		// 动态链接(沙箱内运行;静态链接常因缺静态库/在 box 内失败)
		CompileCmd:          []string{"/usr/bin/gcc", "-O2", "-std=c11", "-o", "{out}", "{in}", "-lm"},
		ToolchainVersionCmd: []string{"/usr/bin/gcc", "--version"},
		// 运行命令相对路径;vertex-sandbox 固定在独立 workspace 中执行。
		RunCmd:     []string{"./{exe}"},
		TimeFactor: 1.0, MemFactor: 1.0, ProcAllow: 8,
		CompilerTimeMs: 10000, CompilerMemKB: 524288,
	},
	"cpp": {
		Name: "cpp", SourceExt: "main.cpp",
		CompileCmd:          []string{"/usr/bin/g++", "-O2", "-std=c++17", "-o", "{out}", "{in}", "-lm"},
		ToolchainVersionCmd: []string{"/usr/bin/g++", "--version"},
		RunCmd:              []string{"./{exe}"},
		TimeFactor:          1.0, MemFactor: 1.0, ProcAllow: 8,
		CompilerTimeMs: 10000, CompilerMemKB: 524288,
	},
	"python": {
		Name: "python", SourceExt: "main.py",
		CompileCmd: nil, // 解释型语言无编译
		RunCmd:     []string{"/usr/bin/python3", "{exe}"},
		TimeFactor: 3.0, MemFactor: 2.0, MemAddKB: 65536, ProcAllow: 32,
		CompilerTimeMs: 10000, CompilerMemKB: 524288,
	},
}

// Result 编译结果。
type Result struct {
	OK        bool
	Error     string // 编译错误输出(有界)
	OutputDir string // 编译产物目录
}

// Compiler 负责在沙箱内编译并缓存产物。
type Compiler struct {
	sandbox *run.Sandbox
	// CacheDir 编译产物缓存目录(worker 本地)
	CacheDir string
	// ScratchDir 每次编译的工作目录
	ScratchDir string
}

func NewCompiler(sandbox *run.Sandbox, cacheDir, scratchDir string) *Compiler {
	return &Compiler{sandbox: sandbox, CacheDir: cacheDir, ScratchDir: scratchDir}
}

// Compile 编译一份源码。返回产物文件路径(在 worker 宿主侧)。
// 缓存键包含语言、编译命令、真实工具链版本和源码哈希。
func (c *Compiler) Compile(ctx context.Context, lang string, source []byte, sourceHash string) (string, *Result) {
	lc, ok := Supported[lang]
	if !ok {
		return "", &Result{OK: false, Error: "unsupported language: " + lang}
	}

	if lc.CompileCmd == nil {
		// 解释型语言:把源码写入 scratch,返回源文件路径
		dir := filepath.Join(c.ScratchDir, "src-"+sourceHash)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", &Result{OK: false, Error: "scratch create: " + err.Error()}
		}
		srcPath := filepath.Join(dir, lc.SourceExt)
		if err := os.WriteFile(srcPath, source, 0o644); err != nil {
			return "", &Result{OK: false, Error: "write source: " + err.Error()}
		}
		return srcPath, &Result{OK: true, OutputDir: dir}
	}

	// 编译型:先查缓存
	toolchainVersion, err := resolveToolchainVersion(ctx, lc.ToolchainVersionCmd)
	if err != nil {
		return "", &Result{OK: false, Error: "toolchain version: " + err.Error()}
	}
	cacheFile := filepath.Join(c.CacheDir,
		cacheFingerprint(lang, lc, sourceHash, toolchainVersion))
	if _, err := os.Stat(cacheFile); err == nil {
		return cacheFile, &Result{OK: true, OutputDir: filepath.Dir(cacheFile)}
	}

	// 编译(沙箱内)
	workDir := filepath.Join(c.ScratchDir, fmt.Sprintf("build-%d-%s", c.sandbox.BoxID, sourceHash))
	_ = os.RemoveAll(workDir)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", &Result{OK: false, Error: "scratch create: " + err.Error()}
	}
	srcPath := filepath.Join(workDir, lc.SourceExt)
	if err := os.WriteFile(srcPath, source, 0o644); err != nil {
		return "", &Result{OK: false, Error: "write source: " + err.Error()}
	}

	compileArgs := make([]string, 0, len(lc.CompileCmd))
	for _, a := range lc.CompileCmd {
		a = strings.ReplaceAll(a, "{in}", lc.SourceExt)
		a = strings.ReplaceAll(a, "{out}", "prog")
		compileArgs = append(compileArgs, a)
	}

	execution := run.Execution{
		Command: compileArgs,
		Limits: run.Limits{
			CPUTime:     time.Duration(lc.CompilerTimeMs) * time.Millisecond,
			WallTime:    time.Duration(lc.CompilerTimeMs) * 2 * time.Millisecond,
			MemoryKB:    lc.CompilerMemKB,
			Processes:   lc.ProcAllow,
			OutputBytes: 8 * 1024 * 1024, // 编译错误输出上限 8MB
		},
	}
	if err := c.sandbox.Reset(); err != nil {
		return "", &Result{OK: false, Error: "sandbox reset: " + err.Error()}
	}
	defer func() { _ = c.sandbox.Reset() }()
	if err := c.sandbox.CopyIn(ctx, map[string]string{lc.SourceExt: srcPath}); err != nil {
		return "", &Result{OK: false, Error: "copy-in source: " + err.Error()}
	}
	res, err := c.sandbox.Execute(ctx, execution)
	if err != nil {
		return "", &Result{OK: false, Error: "compile run failed: " + err.Error()}
	}

	if res.Meta.Status != "" && res.Meta.Status != "RE" {
		// 编译被限制或系统错误 → SE
		v := verdict.FromSandboxMeta(res.Meta)
		return "", &Result{OK: false, Error: "compile " + v + ": " + res.Meta.ExitDescription()}
	}
	if res.Meta.ExitCode != 0 {
		// 编译失败 → CE
		errOut, _ := os.ReadFile(res.StderrPath)
		if len(errOut) > 8192 {
			errOut = errOut[:8192]
		}
		return "", &Result{OK: false, Error: "compile error:\n" + string(errOut)}
	}

	// CopyOut validates the untrusted artifact and atomically publishes it.
	// When another worker wins the immutable cache key race, reuse its file.
	if err := cacheBinary(ctx, c.sandbox, cacheFile); err != nil {
		return "", &Result{OK: false, Error: "cache binary: " + err.Error()}
	}
	return cacheFile, &Result{OK: true, OutputDir: filepath.Dir(cacheFile)}
}

const compileCacheFormat = "vertex-compile-cache-v2"

func cacheFingerprint(language string, config LangConfig, sourceHash, toolchainVersion string) string {
	hash := sha256.New()
	writePart := func(value string) {
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write([]byte(value))
	}
	writePart(compileCacheFormat)
	writePart(language)
	writePart(fmt.Sprintf("compile-command:%d", len(config.CompileCmd)))
	for _, argument := range config.CompileCmd {
		writePart(argument)
	}
	writePart(fmt.Sprintf("toolchain-version-command:%d", len(config.ToolchainVersionCmd)))
	for _, argument := range config.ToolchainVersionCmd {
		writePart(argument)
	}
	writePart(toolchainVersion)
	writePart(sourceHash)
	return hex.EncodeToString(hash.Sum(nil)[:12])
}

func resolveToolchainVersion(ctx context.Context, command []string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("version command is required")
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 4096 {
			message = message[:4096]
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}
	return string(output), nil
}

func cacheBinary(ctx context.Context, sandbox *run.Sandbox, destPath string) error {
	err := sandbox.CopyOut(ctx, "prog", destPath, sandbox.Policy.WorkspaceBytes)
	if err == nil {
		return nil
	}
	info, statErr := os.Lstat(destPath)
	if statErr == nil && info.Mode().IsRegular() {
		return nil
	}
	return err
}
