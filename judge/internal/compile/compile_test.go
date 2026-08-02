package compile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
