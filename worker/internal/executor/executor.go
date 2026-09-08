package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// Case 一个测试点。
type Case struct {
	Index        int
	InputPath    string // 宿主侧输入文件
	ExpectedPath string // 宿主侧期望输出文件(无 SPJ 时)
	TimeLimitMs  int    // 本测试点 CPU 时间(原始,未乘倍率)
	MemLimitKB   int    // 本测试点内存(原始)
	// Dependencies is carried with the case snapshot for batched execution policies.
}

// CaseResult 单测试点判定结果(写入 submission_cases)。
type CaseResult struct {
	CaseIndex     int    `json:"caseIndex"`
	Verdict       string `json:"verdict"`
	TimeMs        int    `json:"timeMs"`
	MemoryKb      int    `json:"memoryKb"`
	ExitStatus    string `json:"exitStatus,omitempty"`
	CheckerOutput string `json:"checkerOutput,omitempty"`
}

// Executor 执行一次提交的全部测试点。
type Executor struct {
	sandbox    *run.Client
	scratchDir string // 每次判题的工作目录(宿主机侧)
	// checkerRunner and checkerSource are only needed by testlib snapshots;
	// a worker without them still judges diff-compared problems.
	checkerRunner *checker.Runner
	checkerSource *checker.SourceCompiler
}

func NewExecutor(
	sandbox *run.Client, scratchDir string,
	checkerRunner *checker.Runner, checkerSource *checker.SourceCompiler,
) *Executor {
	return &Executor{
		sandbox: sandbox, scratchDir: scratchDir,
		checkerRunner: checkerRunner, checkerSource: checkerSource,
	}
}

// Judge 判定一份已编译产物,返回逐测试点结果(短路:失败点后续标 Skipped)。
// langCfg 为语言配置(倍率),exePath 为编译产物(解释型为源码),cases 为测试点。
// Judge 逐个执行测试点。progress 可以为 nil;非 nil 时每判完一个测试点
// 就以已完成数量回调一次,供上层上报判题进度。
func (e *Executor) Judge(
	ctx context.Context, langCfg compile.LangConfig, exePath string,
	cases []Case, grader Grader, progress func(done int),
) ([]CaseResult, int64, int, error) {
	if grader == nil {
		grader = DiffGrader{}
	}
	results := make([]CaseResult, 0, len(cases))
	var totalTime int64
	peakMem := 0
	aborted := ""

	for _, c := range cases {
		if aborted != "" {
			results = append(results, CaseResult{
				CaseIndex: c.Index, Verdict: verdict.Skip,
			})
			continue
		}

		res := e.runOne(ctx, langCfg, exePath, c, grader)
		results = append(results, res)
		totalTime += int64(res.TimeMs)
		if res.MemoryKb > peakMem {
			peakMem = res.MemoryKb
		}
		if res.Verdict != verdict.AC {
			aborted = res.Verdict
		}
		if progress != nil {
			progress(len(results))
		}
	}
	return results, totalTime, peakMem, nil
}

// runOne 执行单个测试点并判定。
func (e *Executor) runOne(
	ctx context.Context, langCfg compile.LangConfig, exePath string, c Case, grader Grader,
) CaseResult {
	env, err := e.sandbox.Create(ctx, run.EnvironmentPolicy{
		MemoryKB:  int(float64(c.MemLimitKB)*langCfg.MemFactor) + langCfg.MemAddKB,
		Processes: langCfg.ProcAllow,
	})
	if err != nil {
		return CaseResult{CaseIndex: c.Index, Verdict: verdict.SE, ExitStatus: err.Error()}
	}
	defer env.Close()
	workDir, err := os.MkdirTemp(e.scratchDir, "judge-")
	if err != nil {
		return CaseResult{CaseIndex: c.Index, Verdict: verdict.SE, ExitStatus: err.Error()}
	}
	defer os.RemoveAll(workDir)
	// 复制输入进 box
	copyIn := map[string]run.InputFile{"input.txt": {Path: c.InputPath}}
	// 复制编译产物进 box
	exeBoxName := "prog"
	if langCfg.CompileCmd == nil {
		// 解释型:源码直接用 {exe} 路径,需复制源码进 box
		exeBoxName = langCfg.SourceExt
		copyIn[exeBoxName] = run.InputFile{Path: exePath, Executable: langCfg.CompileCmd != nil}
	} else {
		copyIn[exeBoxName] = run.InputFile{Path: exePath, Executable: langCfg.CompileCmd != nil}
	}
	if err := env.PutFiles(ctx, copyIn); err != nil {
		return CaseResult{CaseIndex: c.Index, Verdict: verdict.SE, ExitStatus: "copy-in: " + err.Error()}
	}

	// 组装运行命令:把 {exe} 替换为 box 内路径
	runArgs := make([]string, 0, len(langCfg.RunCmd))
	for _, a := range langCfg.RunCmd {
		a = strings.ReplaceAll(a, "{exe}", exeBoxName)
		runArgs = append(runArgs, a)
	}

	cpuLimit := time.Duration(float64(c.TimeLimitMs) * langCfg.TimeFactor * float64(time.Millisecond))
	execution := run.Execution{
		Command:    runArgs,
		StdoutPath: filepath.Join(workDir, "stdout"),
		StdinFile:  "input.txt",
		Limits: run.Limits{
			CPUTime:     cpuLimit,
			WallTime:    cpuLimit * 2,
			MemoryKB:    int(float64(c.MemLimitKB)*langCfg.MemFactor) + langCfg.MemAddKB,
			Processes:   langCfg.ProcAllow,
			OutputBytes: checker.MaxOutputBytes, // 输出上限 32MB,超限按 OLE
		},
	}

	res, err := env.Run(ctx, execution)
	if err != nil {
		return CaseResult{CaseIndex: c.Index, Verdict: verdict.SE, ExitStatus: "run failed: " + err.Error()}
	}

	cr := CaseResult{
		CaseIndex:  c.Index,
		TimeMs:     int(res.Meta.Time * 1000),
		MemoryKb:   res.Meta.EffectiveMemoryKB(),
		ExitStatus: res.Meta.ExitDescription(),
	}

	// 判定:沙箱状态 → 输出比对
	v := verdict.FromSandboxMeta(res.Meta)
	switch {
	case v == verdict.MLE:
		cr.Verdict = verdict.MLE
	case v == verdict.TLE:
		cr.Verdict = verdict.TLE
	case v == verdict.RE:
		cr.Verdict = verdict.RE
	case v == verdict.SE:
		cr.Verdict = verdict.SE
	case v != "":
		cr.Verdict = v
	default:
		// 程序正常退出:交给 grader 判定
		// OLE 检测:若输出文件大小达到上限(沙箱已截断),判 OLE
		if isOutputTruncated(res.StdoutPath, execution.Limits.OutputBytes) {
			cr.Verdict = verdict.OLE
			cr.CheckerOutput = "output size limit exceeded"
			break
		}
		cr.Verdict, cr.CheckerOutput = grader.Grade(ctx, c.InputPath, res.StdoutPath, c.ExpectedPath)
	}

	return cr
}

func isOutputTruncated(stdoutPath string, limitBytes int64) bool {
	fi, err := os.Stat(stdoutPath)
	if err != nil {
		return false
	}
	return fi.Size() >= limitBytes
}
