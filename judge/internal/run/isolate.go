package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vertex-oj/judge/internal/verdict"
)

// Limits 一次运行的资源限制(按语言倍率换算后传入)。
type Limits struct {
	TimeMs      int   // CPU 时间(ms)
	WallTimeMs  int   // 墙钟时间(ms),至少 = 2×CPU
	MemoryKB    int   // 内存限制(KB),cgroup v2
	Processes   int   // 进程/线程数上限
	OutputBytes int64 // stdout 输出上限(字节)
	StackKB     int   // 栈大小(KB)
}

// Config isolate 运行配置。
type Config struct {
	// StdinPath box 内文件路径,作为程序 stdin(如 /box/input.txt)
	StdinPath string
	// Chdir box 内工作目录(默认 /box;设空则用 isolate 默认)
	Chdir string
	// Net 是否共享网络(默认 false=断网)
	Net bool
	// TimeLimitSec CPU 时间上限(秒)
	TimeLimitSec float64
	// WallLimitSec 墙钟上限(秒)
	WallLimitSec float64
	// MemLimitKB 内存上限(KB)
	MemLimitKB int
	// Processes 进程数上限
	Processes int
	// OutputBytes stdout 上限(字节)
	OutputBytes int64
	// StackKB 栈上限
	StackKB int
	// Env 传给沙箱内程序的环境变量(白名单,不要传宿主环境)
	Env []string
}

// RunResult isolate 运行结果。
type RunResult struct {
	Meta       *verdict.IsolateMeta
	StdoutPath string
	StderrPath string
	RunTimeMs  int
}

// Isolate 封装 ioi/isolate CLI。要求:
//   - isolate 二进制在 PATH 中
//   - 宿主为 Linux,支持 cgroups v2(--cg 模式)
//   - worker 容器 unprivileged(此包不做特权操作)
type Isolate struct {
	BoxID int
	// BaseDir isolate box 的父目录(通常是 /var/local/lib/isolate)
	BaseDir string
}

// NewIsolate 创建 isolate 封装,boxID 全局唯一。
func NewIsolate(boxID int, baseDir string) *Isolate {
	if baseDir == "" {
		baseDir = "/var/local/lib/isolate"
	}
	return &Isolate{BoxID: boxID, BaseDir: baseDir}
}

// BoxPath 返回 box 内文件在宿主上的路径。
// isolate --init 在 <base>/<boxid>/ 下创建 box/ 子目录作为沙箱工作目录,
// 编译产物/输入文件都放在这个 box/ 目录里。
func (is *Isolate) BoxPath(name string) string {
	return filepath.Join(is.BoxDir(), "box", name)
}

// BoxDir 返回 box 目录。
func (is *Isolate) BoxDir() string {
	return fmt.Sprintf("%s/%d", is.BaseDir, is.BoxID)
}

// Init 初始化 box(--init),创建目录与挂载命名空间。
func (is *Isolate) Init(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "isolate", "--box-id", itoa(is.BoxID), "--cg", "--init")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("isolate --init: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Cleanup 清理 box(--cleanup),杀掉残留进程并删除目录。
func (is *Isolate) Cleanup(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "isolate", "--box-id", itoa(is.BoxID), "--cg", "--cleanup")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 清理失败不该阻塞判题;记录即可
		return fmt.Errorf("isolate --cleanup: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CopyIn 把文件从宿主复制进 box。data: map[box文件名]宿主路径。
// isolate 没有 --copy-in 顶层选项;box 目录在宿主侧可直接写
// (worker 进程是 root,与 isolate 共享文件系统),直接复制文件即可。
func (is *Isolate) CopyIn(ctx context.Context, data map[string]string) error {
	boxDir := filepath.Join(is.BoxDir(), "box")
	for boxName, hostPath := range data {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		src, err := os.Open(hostPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", hostPath, err)
		}
		dstPath := filepath.Join(boxDir, boxName)
		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			return fmt.Errorf("create %s: %w", dstPath, err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return fmt.Errorf("copy %s -> %s: %w", hostPath, dstPath, err)
		}
		src.Close()
		if err := dst.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Run 在 box 内执行命令。cmdArgs 为 box 内相对路径(如 ./main)或 /bin/... 绝对路径。
// stdout/stderr 经 isolate 的 --stdout/--stderr 定向到 box 内(降权后唯一可写目录);
// worker 侧再从 box 目录读回(见 RunResult.StdoutPath/StderrPath)。
func (is *Isolate) Run(ctx context.Context, cfg *Config, cmdArgs ...string) (*RunResult, error) {
	// meta 与输出文件都写进 box 内 --stdout/--stderr 是 chroot 内路径,
	// worker 侧用宿主侧 BoxPath() 读回。
	meta := is.MetaFilePath()
	_ = os.Remove(meta)

	boxDir := filepath.Join(is.BoxDir(), "box")
	stdoutName := "output.out"
	stderrName := "output.err"
	// 宿主侧路径(worker 读回用)
	stdoutHost := filepath.Join(boxDir, stdoutName)
	stderrHost := filepath.Join(boxDir, stderrName)
	// chroot 内路径(isolate --run 在 chroot 后 open 重定向目标)
	stdoutBox := "/box/" + stdoutName
	stderrBox := "/box/" + stderrName
	_ = os.Remove(stdoutHost)
	_ = os.Remove(stderrHost)

	args := []string{
		"--box-id", itoa(is.BoxID),
		"--cg",
		"--meta", meta,
		"--time", fmt.Sprintf("%.3f", cfg.TimeLimitSec),
		"--wall-time", fmt.Sprintf("%.3f", cfg.WallLimitSec),
		"--mem", itoa(cfg.MemLimitKB),
		// --processes 的值是可选参数,getopt 只在 --processes=N 形式下绑定该值。
		"--processes=" + itoa(cfg.Processes),
		"--fsize", itoa(int(cfg.OutputBytes / 1024)), // KB
	}
	if cfg.StackKB > 0 {
		args = append(args, "--stack", itoa(cfg.StackKB))
	}
	// 断网:isolate 默认不共享网络(loopback only)
	if cfg.Net {
		args = append(args, "--share-net")
	}
	// 工作目录固定为 /box(box 内挂载点),避免依赖 isolate 默认 cwd
	if cfg.Chdir == "" {
		cfg.Chdir = "/box"
	}
	args = append(args, "--chdir", cfg.Chdir)
	if cfg.StdinPath != "" {
		args = append(args, "--stdin", cfg.StdinPath)
	}
	args = append(args, "--stdout", stdoutBox, "--stderr", stderrBox)

	// 环境白名单(剥掉 LD_PRELOAD 等宿主变量)
	env := []string{"PATH=/usr/bin:/bin", "LANG=C"}
	for _, e := range cfg.Env {
		env = append(env, e)
	}
	for _, e := range env {
		args = append(args, "--env="+e)
	}
	fullArgs := append(args, "--run", "--")
	fullArgs = append(fullArgs, cmdArgs...)
	cmd := exec.CommandContext(ctx, "isolate", fullArgs...)
	cmd.Env = env
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		// isolate 本身失败(非目标程序失败)——如果 meta 存在且能解析,按程序失败处理
		if _, statErr := os.Stat(meta); statErr == nil {
			m, perr := verdict.ParseIsolateMeta(meta)
			if perr == nil {
				return &RunResult{Meta: m, StdoutPath: stdoutHost, StderrPath: stderrHost, RunTimeMs: int(elapsed)}, nil
			}
		}
		return nil, fmt.Errorf("isolate --run: %w: %s", err, stderrBuf.String())
	}

	m, parseErr := verdict.ParseIsolateMeta(meta)
	if parseErr != nil {
		// meta 丢失是系统级错误(SE)
		m = &verdict.IsolateMeta{Status: "XX", Message: "meta file missing"}
	}
	return &RunResult{Meta: m, StdoutPath: stdoutHost, StderrPath: stderrHost, RunTimeMs: int(elapsed)}, nil
}

// MetaFilePath 返回 meta 文件路径。
// 放 box 的 box/ 工作目录内:isolate --init 只把 box/ 子目录 chown 给调用者,
// box 根目录仍是 root 所有(0755),isolate 降权后写不进。
func (is *Isolate) MetaFilePath() string {
	return filepath.Join(is.BoxDir(), "box", "meta")
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
