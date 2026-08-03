package run

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	inheritedStdinFD  = 3
	inheritedStdoutFD = 4
)

// streamProcess is an asynchronously running native sandbox whose protocol
// traffic uses inherited pipes. It is intentionally private: callers must use
// a broker that continuously drains output and enforces the byte limit.
type streamProcess struct {
	sandbox *Sandbox
	cmd     *exec.Cmd
	stdin   *os.File
	stdout  *os.File
	stderr  bytes.Buffer
	started time.Time

	waitOnce sync.Once
	result   *Result
	waitErr  error
}

func (s *Sandbox) startStreaming(ctx context.Context, execution Execution) (*streamProcess, error) {
	args, err := s.executionArgs(execution, inheritedStdinFD, inheritedStdoutFD)
	if err != nil {
		return nil, err
	}

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create sandbox stdin pipe: %w", err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		stdinRead.Close()
		stdinWrite.Close()
		return nil, fmt.Errorf("create sandbox stdout pipe: %w", err)
	}

	process := &streamProcess{sandbox: s, stdin: stdinWrite, stdout: stdoutRead}
	cmd := exec.CommandContext(ctx, "vertex-sandbox", args...)
	cmd.Env = runnerEnvironment()
	cmd.ExtraFiles = []*os.File{stdinRead, stdoutWrite}
	cmd.Stderr = &process.stderr
	configureCancellation(cmd)
	process.cmd = cmd
	process.started = time.Now()
	if err := cmd.Start(); err != nil {
		stdinRead.Close()
		stdinWrite.Close()
		stdoutRead.Close()
		stdoutWrite.Close()
		return nil, fmt.Errorf("start vertex-sandbox stream: %w", err)
	}
	// Only the runner may retain the child-facing pipe ends. Otherwise EOF
	// would be delayed until the trusted worker itself exits.
	stdinRead.Close()
	stdoutWrite.Close()
	return process, nil
}

func (p *streamProcess) wait() (*Result, error) {
	p.waitOnce.Do(func() {
		err := p.cmd.Wait()
		elapsed := time.Since(p.started).Milliseconds()
		p.result, p.waitErr = p.sandbox.resultAfterWait(err, p.stderr.String(), elapsed)
	})
	return p.result, p.waitErr
}

func (p *streamProcess) closeIO() {
	_ = p.stdin.Close()
	_ = p.stdout.Close()
}
