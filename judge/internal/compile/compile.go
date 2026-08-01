package compile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vertex-oj/judge/internal/run"
	"github.com/vertex-oj/judge/internal/verdict"
)

// LangConfig 一种语言的编译与运行配置。
type LangConfig struct {
	Name           string
	SourceExt      string   // 源码文件名扩展
	CompileCmd     []string // 编译命令;{in} 替换为源码路径,{out} 替换为输出文件
	RunCmd         []string // 运行命令;{exe} 替换为编译产物路径
	TimeFactor     float64  // CPU 时间倍率
	MemFactor      float64  // 内存倍率
	MemAddKB       int      // 额外内存(KB),如 JVM 基础开销
	ProcAllow      int      // 允许的进程/线程数
	CompilerTimeMs int      // 编译自身的 CPU 时间上限
	CompilerMemKB  int      // 编译自身的内存上限
}

// Supported 判题语言注册表。
// 新增语言:在此加一条 + isolate 的 box 内需有对应工具链。
var Supported = map[string]LangConfig{
	"c": {
		Name: "c", SourceExt: "main.c",
		// 动态链接(沙箱内运行;静态链接常因缺静态库/在 box 内失败)
		CompileCmd: []string{"/usr/bin/gcc", "-O2", "-std=c11", "-o", "{out}", "{in}", "-lm"},
		// 运行命令相对路径;isolate --run 强制 --chdir=/box
		RunCmd:     []string{"./{exe}"},
		TimeFactor: 1.0, MemFactor: 1.0, ProcAllow: 8,
		CompilerTimeMs: 10000, CompilerMemKB: 524288,
	},
	"cpp": {
		Name: "cpp", SourceExt: "main.cpp",
		CompileCmd: []string{"/usr/bin/g++", "-O2", "-std=c++17", "-o", "{out}", "{in}", "-lm"},
		RunCmd:     []string{"./{exe}"},
		TimeFactor: 1.0, MemFactor: 1.0, ProcAllow: 8,
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
	isolate *run.Isolate
	// CacheDir 编译产物缓存目录(worker 本地)
	CacheDir string
	// ScratchDir 每次编译的工作目录
	ScratchDir string
}

func NewCompiler(isolate *run.Isolate, cacheDir, scratchDir string) *Compiler {
	return &Compiler{isolate: isolate, CacheDir: cacheDir, ScratchDir: scratchDir}
}

// Compile 编译一份源码。返回产物文件路径(在 worker 宿主侧)。
// 已按 (lang, sha256) 命中缓存则直接返回缓存路径。
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
	cacheFile := filepath.Join(c.CacheDir, lang+"-"+sourceHash)
	if _, err := os.Stat(cacheFile); err == nil {
		return cacheFile, &Result{OK: true, OutputDir: filepath.Dir(cacheFile)}
	}

	// 编译(沙箱内)
	workDir := filepath.Join(c.ScratchDir, fmt.Sprintf("build-%d-%s", c.isolate.BoxID, sourceHash))
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
		a = strings.ReplaceAll(a, "{in}", "/box/"+lc.SourceExt)
		a = strings.ReplaceAll(a, "{out}", "/box/prog")
		compileArgs = append(compileArgs, a)
	}

	cfg := &run.Config{
		TimeLimitSec: float64(lc.CompilerTimeMs) / 1000.0,
		WallLimitSec: float64(lc.CompilerTimeMs) / 1000.0 * 2,
		MemLimitKB:   lc.CompilerMemKB,
		Processes:    lc.ProcAllow,
		OutputBytes:  8 * 1024 * 1024, // 编译错误输出上限 8MB
	}
	if err := c.isolate.CopyIn(ctx, map[string]string{lc.SourceExt: srcPath}); err != nil {
		return "", &Result{OK: false, Error: "copy-in source: " + err.Error()}
	}
	res, err := c.isolate.Run(ctx, cfg, compileArgs...)
	if err != nil {
		return "", &Result{OK: false, Error: "compile run failed: " + err.Error()}
	}

	if res.Meta.Status != "" && res.Meta.Status != "RE" {
		// 编译被限制或系统错误 → SE
		v := verdict.FromIsolateMeta(res.Meta)
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

	// 编译成功:把 /box/prog 复制出来并缓存
	progPath := filepath.Join(workDir, "prog")
	if err := c.copyOut(ctx, "/box/prog", progPath); err != nil {
		return "", &Result{OK: false, Error: "copy-out binary: " + err.Error()}
	}
	if err := os.Rename(progPath, cacheFile); err != nil {
		return "", &Result{OK: false, Error: "cache binary: " + err.Error()}
	}
	return cacheFile, &Result{OK: true, OutputDir: filepath.Dir(cacheFile)}
}

// copyOut 把 box 内编译产物复制到宿主侧目标路径。
// worker 容器与宿主共享文件系统,直接读 box 目录(不依赖 --copy-out 的相对路径语义)。
func (c *Compiler) copyOut(ctx context.Context, boxPath, destPath string) error {
	_ = ctx
	_ = boxPath
	src := c.isolate.BoxPath("prog")
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0o755)
}
