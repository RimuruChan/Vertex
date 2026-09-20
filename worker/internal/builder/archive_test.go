package builder

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadHeadTruncatesAtRuneBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	// "中文" is two three-byte runes; a five-byte read must not split the second.
	if err := os.WriteFile(path, []byte("中文"), 0o644); err != nil {
		t.Fatal(err)
	}
	head := readHead(path, 5)
	if !bytes.HasPrefix([]byte(head), []byte("中")) {
		t.Fatalf("head = %q, want it to start with a whole rune", head)
	}
	for _, char := range head {
		if char == '�' {
			t.Fatalf("head = %q contains a broken rune", head)
		}
	}
}

func TestReadHeadMarksTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, bytes.Repeat([]byte("a"), 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if head := readHead(path, 10); head != "aaaaaaaaaa\n…" {
		t.Fatalf("head = %q", head)
	}
	if head := readHead(path, 100); head != string(bytes.Repeat([]byte("a"), 100)) {
		t.Fatalf("complete read was marked as truncated: %q", head)
	}
}
