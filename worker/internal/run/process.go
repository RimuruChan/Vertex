package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const inheritedStdinFD, inheritedStdoutFD = 4, 5

// Process owns one execution, including descendants. File outputs are copied
// to caller-owned destinations by Wait; log previews are capped at 8 KiB.
type Process struct {
	Stdin           io.WriteCloser
	Stdout          io.ReadCloser
	env             *Environment
	execution       Execution
	cmd             *exec.Cmd
	stderr          bytes.Buffer
	cancel          context.CancelFunc
	ctx             context.Context
	stopEnvironment func() bool
	started         time.Time
	waitOnce        sync.Once
	result          *Result
	err             error
	streamBytes     atomic.Int64
	streamExceeded  atomic.Bool
}

func (env *Environment) Start(ctx context.Context, execution Execution) (*Process, error) {
	env.mu.Lock()
	defer env.mu.Unlock()
	if err := env.available(); err != nil {
		return nil, err
	}
	if execution.Limits.MemoryKB > env.policy.MemoryKB || execution.Limits.Processes > env.policy.Processes {
		return nil, fmt.Errorf("execution exceeds environment resource budget")
	}
	if execution.Stream && execution.StdoutPath != "" {
		return nil, fmt.Errorf("streamed stdout cannot also be exported")
	}
	// A rejected start must never reuse a previous execution's metadata.
	if err := os.Remove(env.box.MetaFilePath()); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	stdinFD, stdoutFD := -1, -1
	if execution.Stream {
		stdinFD, stdoutFD = inheritedStdinFD, inheritedStdoutFD
	}
	args, err := env.box.executionArgs(execution, stdinFD, stdoutFD)
	if err != nil {
		return nil, err
	}
	args = append([]string{"start", "--lease-fd", "3"}, args[1:]...)
	runCtx, cancel := context.WithCancel(ctx)
	process := &Process{env: env, execution: execution, ctx: runCtx, cancel: cancel, started: time.Now()}
	process.stopEnvironment = context.AfterFunc(env.ctx, cancel)
	cmd := exec.CommandContext(runCtx, "vertex-sandbox", args...)
	cmd.Env = runnerEnvironment()
	cmd.ExtraFiles = []*os.File{env.lease}
	cmd.Stderr = &process.stderr
	configureCancellation(cmd)
	process.cmd = cmd
	var childInput, childOutput *os.File
	if execution.Stream {
		var inputWrite, outputRead *os.File
		childInput, inputWrite, err = os.Pipe()
		if err == nil {
			outputRead, childOutput, err = os.Pipe()
		}
		if err != nil {
			if childInput != nil {
				childInput.Close()
				inputWrite.Close()
			}
			cancel()
			process.stopEnvironment()
			return nil, err
		}
		process.Stdin = inputWrite
		process.Stdout = &boundedOutput{ReadCloser: outputRead, process: process, limit: execution.Limits.OutputBytes}
		cmd.ExtraFiles = append(cmd.ExtraFiles, childInput, childOutput)
		defer childInput.Close()
		defer childOutput.Close()
	}
	if err := cmd.Start(); err != nil {
		process.closeIO()
		cancel()
		process.stopEnvironment()
		return nil, err
	}
	env.active = process
	if execution.Stream {
		env.streams = append(env.streams, process)
	}
	return process, nil
}

func (process *Process) Cancel() { process.cancel() }

func preview(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, _ := io.ReadAll(io.LimitReader(file, 8192))
	return string(data)
}

func (process *Process) Wait() (*Result, error) {
	process.waitOnce.Do(func() {
		defer process.cancel()
		defer process.stopEnvironment()
		defer func() { process.env.mu.Lock(); process.env.active = nil; process.env.mu.Unlock() }()
		err := process.cmd.Wait()
		result, resultErr := process.env.box.resultAfterWait(err, process.stderr.String(), time.Since(process.started).Milliseconds())
		if resultErr != nil && process.ctx.Err() != nil {
			// Cancellation can reach the CLI before it installs signal handlers.
			result = &Result{Meta: &Meta{Status: "XX", TerminationReason: TerminationCancelled,
				Killed: true, Message: process.ctx.Err().Error()}}
			resultErr = nil
		}
		process.result, process.err = result, resultErr
		if result == nil {
			return
		}
		result.Stdout, result.Stderr = preview(result.StdoutPath), preview(result.StderrPath)
		if process.execution.Stream {
			result.Meta.StdoutBytes = process.streamBytes.Load()
			if process.streamExceeded.Load() {
				result.Meta.OutputLimit = true
				result.Meta.TerminationReason = TerminationOutputLimit
			}
		}
		internalOut, internalErr := result.StdoutPath, result.StderrPath
		result.StdoutPath, result.StderrPath = "", ""
		for _, output := range []struct {
			source, target string
			stdout         bool
		}{
			{internalOut, process.execution.StdoutPath, true}, {internalErr, process.execution.StderrPath, false},
		} {
			if output.target == "" || output.source == "" {
				continue
			}
			if output.stdout && process.execution.Stream {
				process.err = errors.Join(process.err, fmt.Errorf("streamed stdout cannot also be exported"))
				continue
			}
			if err := copyArtifact(context.Background(), output.source, output.target, process.execution.Limits.OutputBytes); err != nil {
				process.err = errors.Join(process.err, err)
			} else if output.stdout {
				result.StdoutPath = output.target
			} else {
				result.StderrPath = output.target
			}
		}
	})
	return process.result, process.err
}

func (process *Process) closeIO() {
	if process.Stdin != nil {
		_ = process.Stdin.Close()
	}
	if process.Stdout != nil {
		_ = process.Stdout.Close()
	}
}

type boundedOutput struct {
	io.ReadCloser
	process *Process
	limit   int64
}

func (reader *boundedOutput) Read(data []byte) (int, error) {
	n, err := reader.ReadCloser.Read(data)
	if reader.process.streamBytes.Add(int64(n)) >= reader.limit {
		reader.process.streamExceeded.Store(true)
		reader.process.Cancel()
		return n, &streamLimitError{limit: reader.limit}
	}
	return n, err
}
