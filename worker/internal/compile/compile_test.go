package compile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RimuruChan/Vertex/worker/internal/run"
)

func TestCacheFingerprintIncludesCommandAndToolchainVersion(t *testing.T) {
	base := LangConfig{
		CompileCmd:          []string{"/usr/bin/g++", "-O2", "{in}", "-o", "{out}"},
		ToolchainVersionCmd: []string{"/usr/bin/g++", "--version"},
	}
	original := cacheFingerprint("cpp", base, "source-hash", "g++ 14.2\n")
	if len(original) != 24 {
		t.Fatalf("fingerprint length = %d, want 24", len(original))
	}
	if again := cacheFingerprint("cpp", base, "source-hash", "g++ 14.2\n"); again != original {
		t.Fatalf("fingerprint is not stable: %q != %q", again, original)
	}

	changedCommand := base
	changedCommand.CompileCmd = append([]string(nil), base.CompileCmd...)
	changedCommand.CompileCmd[1] = "-O3"
	if got := cacheFingerprint("cpp", changedCommand, "source-hash", "g++ 14.2\n"); got == original {
		t.Fatal("compile command change did not change cache fingerprint")
	}
	if got := cacheFingerprint("cpp", base, "source-hash", "g++ 15.1\n"); got == original {
		t.Fatal("toolchain version change did not change cache fingerprint")
	}
	shiftedBoundary := base
	shiftedBoundary.CompileCmd = append(append([]string(nil), base.CompileCmd...), base.ToolchainVersionCmd...)
	shiftedBoundary.ToolchainVersionCmd = nil
	if got := cacheFingerprint("cpp", shiftedBoundary, "source-hash", "g++ 14.2\n"); got == original {
		t.Fatal("command section boundary did not change cache fingerprint")
	}
}

func TestCacheBinaryCopiesAndKeepsFirstPublisher(t *testing.T) {
	sandbox := run.NewSandbox(7, t.TempDir())
	cacheDir := t.TempDir()
	source := sandbox.BoxPath("prog")
	dest := filepath.Join(cacheDir, "cpp-hash")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(source, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cacheBinary(context.Background(), sandbox, dest); err != nil {
		t.Fatalf("cache first binary: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source should remain after copy: %v", err)
	}

	if err := os.WriteFile(source, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cacheBinary(context.Background(), sandbox, dest); err != nil {
		t.Fatalf("reuse cached binary: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first" {
		t.Fatalf("cached content = %q, want immutable first publisher", got)
	}
}
