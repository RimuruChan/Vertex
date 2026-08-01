package checker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vertex-oj/judge/internal/verdict"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckDiff(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		actual   string
		expected string
		want     string
	}{
		{"exact match", "42\n", "42\n", verdict.AC},
		{"trailing whitespace ignored", "42  \n", "42\n", verdict.AC},
		{"trailing newline difference", "42", "42\n", verdict.AC},
		{"extra trailing blank lines ignored", "42\n\n\n", "42\n", verdict.AC},
		{"different value", "43\n", "42\n", verdict.WA},
		{"completely different", "hello\n", "world\n", verdict.WA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := write(t, dir, "actual.out", tt.actual)
			expected := write(t, dir, "expected.out", tt.expected)
			got, _, err := CheckDiff(actual, expected)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("CheckDiff() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckDiffMissingFiles(t *testing.T) {
	dir := t.TempDir()
	// 实际输出文件缺失 → SE
	got, _, err := CheckDiff(filepath.Join(dir, "missing.out"), filepath.Join(dir, "exp.out"))
	if err != nil {
		t.Fatal(err)
	}
	if got != verdict.SE {
		t.Errorf("missing actual = %q, want SE", got)
	}
}
