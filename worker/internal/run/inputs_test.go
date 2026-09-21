package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRootedInputs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyInputs(context.Background(), root, map[string]string{"src/main.cpp": source, "src/include/header.h": source}, 20); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "src/include/header.h")); err != nil || string(data) != "content" {
		t.Fatalf("nested copy: %q %v", data, err)
	}
	if err := copyInputs(context.Background(), root, map[string]string{"a": source, "b": source}, 10); err == nil {
		t.Fatal("aggregate workspace budget was bypassed")
	}
	for _, name := range []string{"../outside", "/absolute", "C:/absolute", "a/../b", `a\b`, "a//b", "a/./b", "."} {
		if ValidateInputPath(name) == nil {
			t.Fatalf("unsafe input path accepted: %q", name)
		}
	}
}

func TestRootedInputRejectsEscapingParentLink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := copyInputs(context.Background(), root, map[string]string{"link/secret": source}, 100); err == nil {
		t.Fatal("copy escaped through parent symlink")
	}
	if data, err := os.ReadFile(filepath.Join(outside, "secret")); err != nil || string(data) != "preserve" {
		t.Fatal("external file was changed")
	}
	if err := chmodInput(root, "link/secret"); err == nil {
		t.Fatal("chmod escaped through parent symlink")
	}
}
