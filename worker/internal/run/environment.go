package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Client contains defaults only. Native sandbox code allocates all resources.
type Client struct {
	baseDir string
	Policy  Policy
}

func NewClient(baseDir string, policy Policy) *Client {
	if baseDir == "" {
		baseDir = "/vertex/sandbox"
	}
	return &Client{baseDir: baseDir, Policy: policy}
}

type EnvironmentPolicy struct {
	MemoryKB  int
	Processes int
}

type InputFile struct {
	Path       string
	Executable bool
}

// Environment retains files across sequential executions. Only one Start may
// be active at a time; independent environments can run concurrently.
type Environment struct {
	box        *box
	lease      *os.File
	policy     EnvironmentPolicy
	ctx        context.Context
	cancel     context.CancelFunc
	stopParent func() bool
	mu         sync.Mutex
	active     *Process
	streams    []*Process
	closed     bool
	closeOnce  sync.Once
	closeErr   error
}

func (client *Client) Create(ctx context.Context, policy EnvironmentPolicy) (*Environment, error) {
	if err := client.Policy.validate(); err != nil {
		return nil, err
	}
	if policy.MemoryKB <= 0 || policy.Processes <= 0 {
		return nil, fmt.Errorf("environment memory and process budgets must be positive")
	}
	creation, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	state, lease, err := createNative(creation, client, policy)
	if err != nil {
		return nil, err
	}
	state.Policy = client.Policy
	lifetime, cancelLifetime := context.WithCancel(ctx)
	env := &Environment{box: state, lease: lease, policy: policy, ctx: lifetime, cancel: cancelLifetime}
	env.mu.Lock()
	env.stopParent = context.AfterFunc(ctx, func() { _ = env.Close() })
	env.mu.Unlock()
	if err := ctx.Err(); err != nil {
		_ = env.Close()
		return nil, err
	}
	return env, nil
}

func (env *Environment) available() error {
	if env.closed {
		return fmt.Errorf("sandbox environment is closed")
	}
	if env.active != nil {
		return fmt.Errorf("sandbox environment already has an active execution")
	}
	return env.ctx.Err()
}

func (env *Environment) PutFiles(ctx context.Context, files map[string]InputFile) error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if err := env.available(); err != nil {
		return err
	}
	paths := make(map[string]string, len(files))
	var bytes int64
	for name, file := range files {
		if err := validateBoxName(name); err != nil {
			return err
		}
		info, err := os.Stat(file.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > env.box.Policy.WorkspaceBytes-bytes {
			return fmt.Errorf("input files exceed environment workspace budget or are not regular files")
		}
		bytes += info.Size()
		paths[name] = file.Path
	}
	if err := env.box.CopyIn(ctx, paths); err != nil {
		return err
	}
	for name, file := range files {
		if file.Executable {
			if err := os.Chmod(env.box.BoxPath(name), 0755); err != nil {
				return err
			}
		}
	}
	return nil
}

func (env *Environment) ExportFile(ctx context.Context, name, destination string, maxBytes int64) error {
	env.mu.Lock()
	defer env.mu.Unlock()
	if err := env.available(); err != nil {
		return err
	}
	return env.box.CopyOut(ctx, name, destination, maxBytes)
}

func (env *Environment) Run(ctx context.Context, execution Execution) (*Result, error) {
	if execution.Stream {
		return nil, fmt.Errorf("streaming executions require Start and a pipe consumer")
	}
	process, err := env.Start(ctx, execution)
	if err != nil {
		return nil, err
	}
	return process.Wait()
}

func (env *Environment) Close() error {
	env.closeOnce.Do(func() {
		env.mu.Lock()
		env.closed = true
		active := env.active
		streams := env.streams
		if env.stopParent != nil {
			env.stopParent()
		}
		env.cancel()
		env.mu.Unlock()
		if active != nil {
			active.Cancel()
			_, _ = active.Wait()
		}
		for _, process := range streams {
			process.closeIO()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "vertex-sandbox", "close", "--box-id", itoa(env.box.BoxID),
			"--base", env.box.BaseDir, "--lease-fd", "3")
		cmd.Env = runnerEnvironment()
		cmd.ExtraFiles = []*os.File{env.lease}
		output, err := cmd.CombinedOutput()
		if err != nil {
			err = fmt.Errorf("close sandbox environment: %w: %s", err, output)
		}
		env.closeErr = errors.Join(err, env.lease.Close())
	})
	return env.closeErr
}
