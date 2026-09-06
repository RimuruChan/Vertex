package builder

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGenerateCommandRejectsShellMetacharacters(t *testing.T) {
	name, arguments, err := ParseGenerateCommand("  gen_random 1000  -seed=42 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gen_random" {
		t.Fatalf("name = %q, want gen_random", name)
	}
	if len(arguments) != 2 || arguments[0] != "1000" || arguments[1] != "-seed=42" {
		t.Fatalf("arguments = %v", arguments)
	}

	// The parsed argv goes straight to execve, so anything a shell would treat
	// specially must be rejected even though no shell is involved.
	for _, command := range []string{
		"", "   ", "gen; rm -rf /", "gen $(id)", "gen `id`", "gen 'a b'", "../gen 5", "gen \"x\"",
	} {
		if _, _, err := ParseGenerateCommand(command); err == nil {
			t.Fatalf("ParseGenerateCommand(%q) accepted an unsafe command", command)
		}
	}
}

func TestNormalizeTextEndsWithExactlyOneNewline(t *testing.T) {
	cases := map[string]string{
		"1 2":        "1 2\n",
		"1 2\n":      "1 2\n",
		"a\r\nb":     "a\nb\n",
		"a\rb":       "a\nb\n",
		"":           "",
		"trailing\n": "trailing\n",
	}
	for input, want := range cases {
		if got := string(normalizeText(input)); got != want {
			t.Fatalf("normalizeText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildArchivePacksTestsAndChecker(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"1.in", "1.out", "2.in", "2.out"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	archive, err := buildArchive(workspace, 2, &SourceFile{
		Name: "check", Language: "cpp", SourceCode: "int main(){}",
	})
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	found := map[string]bool{}
	for _, entry := range reader.File {
		found[entry.Name] = true
	}
	for _, want := range []string{"1.in", "1.out", "2.in", "2.out", CheckerFileName} {
		if !found[want] {
			t.Fatalf("archive is missing %s (has %v)", want, found)
		}
	}
	if len(found) != 5 {
		t.Fatalf("archive has unexpected entries: %v", found)
	}
}

func TestBuildArchiveOmitsCheckerForDiffPackages(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"1.in", "1.out"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := buildArchive(workspace, 1, nil)
	if err != nil {
		t.Fatalf("buildArchive: %v", err)
	}
	reader, _ := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if len(reader.File) != 2 {
		t.Fatalf("archive has %d entries, want 2", len(reader.File))
	}
}

func TestBuildArchiveFailsWhenATestIsMissing(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "1.in"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := buildArchive(workspace, 1, nil); err == nil {
		t.Fatal("buildArchive accepted a test without an answer")
	}
}

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

func TestVerdictMatches(t *testing.T) {
	cases := []struct {
		expected, actual string
		want             bool
	}{
		{"", "Wrong Answer", true},
		{"Accepted", "Accepted", true},
		{"Accepted", "Wrong Answer", false},
		{"Wrong Answer", "Wrong Answer", true},
		{"Wrong Answer", "Time Limit Exceeded", false},
		{"Any Rejection", "Time Limit Exceeded", true},
		{"Any Rejection", "Accepted", false},
		{"Presentation Error", "Wrong Answer", true},
	}
	for _, item := range cases {
		if got := verdictMatches(item.expected, item.actual); got != item.want {
			t.Fatalf("verdictMatches(%q, %q) = %v, want %v",
				item.expected, item.actual, got, item.want)
		}
	}
}
