//go:build linux

package run

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("VERTEX_SANDBOX_INTEGRATION") != "1" {
		t.Skip("requires native sandbox in a privileged test container")
	}
	return NewClient("/vertex/sandbox", DefaultPolicy())
}

func environmentFor(t *testing.T, client *Client, memory, processes int) *Environment {
	t.Helper()
	env, err := client.Create(context.Background(), EnvironmentPolicy{MemoryKB: memory, Processes: processes})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Close(); err != nil {
			t.Error(err)
		}
	})
	return env
}

func pythonExecution(source string) Execution {
	return Execution{Command: []string{"/usr/bin/python3", "-u", "-c", source}, Limits: Limits{
		CPUTime: 2 * time.Second, WallTime: 4 * time.Second, MemoryKB: 262144,
		Processes: 8, OutputBytes: 1024 * 1024,
	}}
}

func TestEnvironmentLifecycle(t *testing.T) {
	client := integrationClient(t)
	env := environmentFor(t, client, 262144, 8)
	source := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := env.PutFiles(context.Background(), map[string]InputFile{"input": {Path: source}}); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{
		`import os; assert os.geteuid() != 0; assert open("input").read()=="payload"; open("artifact","w").write("one"); print("first")`,
		`assert open("artifact").read()=="one"; open("artifact","w").write("two"); print("second")`,
	} {
		result, err := env.Run(context.Background(), pythonExecution(code))
		if err != nil || result.Meta.ExitCode != 0 || result.Meta.Status != "" {
			t.Fatalf("run: %v %+v", err, result)
		}
		if result.StdoutPath != "" || result.StderrPath != "" {
			t.Fatal("internal output path escaped the API")
		}
	}
	artifact := filepath.Join(t.TempDir(), "artifact")
	if err := env.ExportFile(context.Background(), "artifact", artifact, 16); err != nil {
		t.Fatal(err)
	}
	root := env.box.BoxDir()
	if err := env.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("environment directory remains: %v", err)
	}
	if data, _ := os.ReadFile(artifact); string(data) != "two" {
		t.Fatal("exported artifact did not survive Close")
	}
	if _, err := env.Start(context.Background(), pythonExecution("pass")); err == nil {
		t.Fatal("Start after Close succeeded")
	}
}

func TestEnvironmentProcesses(t *testing.T) {
	client := integrationClient(t)
	env := environmentFor(t, client, 262144, 8)
	other := environmentFor(t, client, 262144, 8)
	if env.box.BoxID == other.box.BoxID {
		t.Fatal("concurrent environments share an identity")
	}
	process, err := env.Start(context.Background(), pythonExecution("import time; time.sleep(60)"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Start(context.Background(), pythonExecution("pass")); err == nil {
		t.Fatal("concurrent Start in one environment was accepted")
	}
	process.Cancel()
	if _, err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	result, err := env.Run(context.Background(), pythonExecution("print('usable')"))
	if err != nil || strings.TrimSpace(result.Stdout) != "usable" {
		t.Fatalf("environment unusable after cancellation: %v %+v", err, result)
	}
	active, err := env.Start(context.Background(), pythonExecution("import time; time.sleep(60)"))
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := active.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentLimits(t *testing.T) {
	client := integrationClient(t)
	for _, tc := range []struct {
		name, source      string
		memory, processes int
		output            int64
		reason            TerminationReason
	}{
		{"memory", "bytearray(128*1024*1024)", 32768, 8, 1048576, TerminationMemoryLimit},
		{"output", "import os; os.write(1,b'x'*131072)", 262144, 8, 65536, TerminationOutputLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := environmentFor(t, client, tc.memory, tc.processes)
			execution := pythonExecution(tc.source)
			execution.Limits.MemoryKB, execution.Limits.Processes, execution.Limits.OutputBytes = tc.memory, tc.processes, tc.output
			result, err := env.Run(context.Background(), execution)
			if err != nil || result.Meta.TerminationReason != tc.reason {
				t.Fatalf("limit: %v %+v", err, result)
			}
			if len(result.Stdout) > 8192 || len(result.Stderr) > 8192 {
				t.Fatal("log preview is unbounded")
			}
		})
	}
	// Cached file pages survive one execution; the environment's aggregate
	// memory limit must still constrain the next process.
	env := environmentFor(t, client, 65536, 8)
	first := pythonExecution("f=open('retained','wb'); block=b'x'*1048576\nfor _ in range(40): f.write(block)\nf.close()")
	first.Limits.MemoryKB = 65536
	first.Limits.OutputBytes = 64 * 1024 * 1024
	if result, err := env.Run(context.Background(), first); err != nil || result.Meta.Status != "" {
		t.Fatalf("initial execution: %v %+v", err, result)
	}
	second := pythonExecution("bytearray(40*1024*1024)")
	second.Limits.MemoryKB = 65536
	result, err := env.Run(context.Background(), second)
	if err != nil || result.Meta.TerminationReason != TerminationMemoryLimit {
		t.Fatalf("aggregate memory: %v %+v", err, result)
	}
}

func TestEnvironmentDuplex(t *testing.T) {
	client := integrationClient(t)
	left, right := environmentFor(t, client, 262144, 8), environmentFor(t, client, 262144, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := RunDuplex(ctx, left, pythonExecution("print('ping', flush=True); assert input()=='PING'"),
		right, pythonExecution("print(input().upper(), flush=True)"), DuplexOptions{IdleTimeout: 2 * time.Second, TranscriptBytes: 1024})
	if err != nil || result.TerminationReason != TaskCompleted {
		t.Fatalf("duplex: %v %+v", err, result)
	}
	if string(result.Transcript.LeftToRight) != "ping\n" || string(result.Transcript.RightToLeft) != "PING\n" {
		t.Fatalf("transcript: %+v", result.Transcript)
	}
}

func TestEnvironmentLeaseRecovery(t *testing.T) {
	client := integrationClient(t)
	lost, err := client.Create(context.Background(), EnvironmentPolicy{MemoryKB: 262144, Processes: 8})
	if err != nil {
		t.Fatal(err)
	}
	lost.stopParent()
	defer lost.cancel()
	process, err := lost.Start(context.Background(), pythonExecution("import time; time.sleep(60)"))
	if err != nil {
		lost.Close()
		t.Fatal(err)
	}
	// Simulate losing the client capability without issuing Close. The native
	// process retains the lease until it finishes and cleans its descendants.
	if err := lost.lease.Close(); err != nil {
		t.Fatal(err)
	}
	other := environmentFor(t, client, 262144, 8)
	if other.box.BoxID == lost.box.BoxID {
		t.Fatal("identity reused while native process still holds the lease")
	}
	process.Cancel()
	if _, err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	var recovered *Environment
	for i := 0; i <= lost.box.BoxID; i++ {
		candidate := environmentFor(t, client, 262144, 8)
		if candidate.box.BoxID == lost.box.BoxID {
			recovered = candidate
			break
		}
	}
	if recovered == nil {
		t.Fatal("abandoned environment was not reclaimed")
	}
	result, err := recovered.Run(context.Background(), pythonExecution("print('recovered')"))
	if err != nil || strings.TrimSpace(result.Stdout) != "recovered" {
		t.Fatalf("recovered environment unusable: %v %+v", err, result)
	}
}

func TestEnvironmentIsolationAndExport(t *testing.T) {
	client := integrationClient(t)
	env := environmentFor(t, client, 262144, 8)
	execution := pythonExecution(`import os, socket
for path in ["/vertex/testdata", "/vertex/run", "/vertex/cache"]:
    try: open(path)
    except PermissionError: pass
    else: raise AssertionError(path)
try: socket.socket()
except PermissionError: pass
else: raise AssertionError("network allowed")
assert os.geteuid() != 0
assert os.getenv("JUDGE_API_TOKEN") is None
os.symlink("/etc/passwd", "escape")
print("isolated")`)
	result, err := env.Run(context.Background(), execution)
	if err != nil || strings.TrimSpace(result.Stdout) != "isolated" {
		t.Fatalf("isolation: %v %+v", err, result)
	}
	if err := env.ExportFile(context.Background(), "escape", filepath.Join(t.TempDir(), "leak"), 65536); err == nil {
		t.Fatal("symlink export was accepted")
	}
}

func TestEnvironmentStreamLimit(t *testing.T) {
	env := environmentFor(t, integrationClient(t), 262144, 8)
	execution := pythonExecution("import os\nwhile True: os.write(1,b'x'*65536)")
	execution.Stream = true
	execution.Limits.OutputBytes = 65536
	process, err := env.Start(context.Background(), execution)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, process.Stdout); err == nil {
		t.Fatal("stream output limit was not enforced")
	}
	result, err := process.Wait()
	if err != nil || !result.Meta.OutputLimit {
		t.Fatalf("stream limit: %v %+v", err, result)
	}
}
