package domain

import (
	"strings"
	"testing"
)

func TestMergeText(t *testing.T) {
	for _, test := range []struct {
		name, base, local, remote, want string
		ok                              bool
	}{
		{"separate-lines", "a\nb\nc\n", "A\nb\nc\n", "a\nb\nC\n", "A\nb\nC\n", true},
		{"adjacent-replacements", "a\nb\n", "A\nb\n", "a\nB\n", "A\nB\n", true},
		{"same-line", "abc\n", "Abc\n", "abC\n", "", false},
		{"same-insertion", "a\n", "x\na\n", "x\na\n", "x\na\n", true},
		{"different-insertion", "a\n", "x\na\n", "y\na\n", "", false},
		{"insert-at-deletion", "a\nb\n", "b\n", "x\na\nb\n", "", false},
		{"endings", "a\r\nb\r\nc", "A\r\nb\r\nc", "a\r\nb\r\nC", "A\r\nb\r\nC", true},
		{"delete-separate", "a\nb\nc\n", "b\nc\n", "a\nb\nC\n", "b\nC\n", true},
		{"empty-base", "", "a", "b", "", false},
		{"unchanged", "a", "a", "b", "b", true},
		{"binary", "a\x00b", "A\x00b", "a\x00B", "", false},
		{"large", strings.Repeat("a\n", 3000), strings.Repeat("b\n", 3000), strings.Repeat("c\n", 3000), "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := MergeText([]byte(test.base), []byte(test.local), []byte(test.remote))
			if ok != test.ok || ok && string(got) != test.want {
				t.Fatalf("got %q %v, want %q %v", got, ok, test.want, test.ok)
			}
		})
	}
}
