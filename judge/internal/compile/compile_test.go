package compile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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

func TestCacheBinaryCopiesAndReplaces(t *testing.T) {
	sourceDir := t.TempDir()
	cacheDir := t.TempDir()
	source := filepath.Join(sourceDir, "prog")
	dest := filepath.Join(cacheDir, "cpp-hash")

	if err := os.WriteFile(source, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cacheBinary(source, dest); err != nil {
		t.Fatalf("cache first binary: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source should remain after copy: %v", err)
	}

	if err := os.WriteFile(source, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cacheBinary(source, dest); err != nil {
		t.Fatalf("replace cached binary: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("cached content = %q, want %q", got, "second")
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("cached mode = %o, want 755", info.Mode().Perm())
	}
}
