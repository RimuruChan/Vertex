//go:build linux

package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// The native allocator transfers a locked file descriptor, rather than asking
// the worker to choose an execution identity or manage lock-file paths.
func createNative(ctx context.Context, client *Client, policy EnvironmentPolicy) (*box, *os.File, error) {
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	parent := os.NewFile(uintptr(pair[0]), "sandbox-parent")
	child := os.NewFile(uintptr(pair[1]), "sandbox-child")
	defer parent.Close()
	defer child.Close()
	args := []string{"create", "--channel-fd", "3", "--base", client.baseDir,
		"--memory-kb", itoa(policy.MemoryKB), "--processes", itoa(policy.Processes)}
	if client.Policy.CPUSet != "" {
		args = append(args, "--cpu-set", client.Policy.CPUSet)
	}
	cmd := exec.CommandContext(ctx, "vertex-sandbox", args...)
	cmd.Env = runnerEnvironment()
	cmd.ExtraFiles = []*os.File{child}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	child.Close()
	data, control := make([]byte, 16384), make([]byte, syscall.CmsgSpace(4))
	n, controlSize, flags, _, receiveErr := syscall.Recvmsg(pair[0], data, control, syscall.MSG_CMSG_CLOEXEC)
	waitErr := cmd.Wait()
	var descriptors []int
	messages, parseErr := syscall.ParseSocketControlMessage(control[:controlSize])
	if parseErr == nil {
		for _, message := range messages {
			fds, err := syscall.ParseUnixRights(&message)
			if err != nil {
				parseErr = err
				break
			}
			descriptors = append(descriptors, fds...)
		}
	}
	keep := false
	defer func() {
		if !keep {
			for _, fd := range descriptors {
				syscall.Close(fd)
			}
		}
	}()
	if waitErr != nil {
		return nil, nil, fmt.Errorf("create sandbox: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if receiveErr != nil {
		return nil, nil, receiveErr
	}
	if parseErr != nil || flags&(syscall.MSG_TRUNC|syscall.MSG_CTRUNC) != 0 || len(descriptors) != 1 {
		return nil, nil, fmt.Errorf("invalid sandbox capability response")
	}
	var result struct {
		Base string `json:"base"`
		Slot int    `json:"slot"`
	}
	if err := json.Unmarshal(data[:n], &result); err != nil {
		return nil, nil, err
	}
	if !filepath.IsAbs(result.Base) || result.Slot < 0 || result.Slot > 4095 {
		return nil, nil, fmt.Errorf("invalid sandbox environment response")
	}
	keep = true
	return newBox(result.Slot, result.Base), os.NewFile(uintptr(descriptors[0]), "sandbox-lease"), nil
}
