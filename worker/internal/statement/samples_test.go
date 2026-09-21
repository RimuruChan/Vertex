package statement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleStagingPreservesBytesWithoutExecutingMarkup(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sample")
	body := []byte("\\input{/secret}\n{{nextsample}}\n")
	if err := os.WriteFile(source, body, 0600); err != nil {
		t.Fatal(err)
	}
	commands, files, err := prepareSamples(root, Options{Title: `Name }\input{secret}`, Samples: []Sample{{Input: source, Answer: source}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(commands, `Name }\input{secret}`) || strings.Contains(commands, "/secret") {
		t.Fatal("untrusted text entered wrapper commands")
	}
	for _, name := range files {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != string(body) {
			t.Fatal("sample bytes changed")
		}
	}
	if err := os.WriteFile(source, []byte{0, 1, 2}, 0600); err != nil {
		t.Fatal(err)
	}
	commands, files, err = prepareSamples(root, Options{Samples: []Sample{{Input: source, Answer: source}}})
	if err != nil || len(files) != 0 || !strings.Contains(commands, "binary sample") {
		t.Fatal("binary sample staged as TeX input")
	}
	if err := os.WriteFile(source, []byte(strings.Repeat("x", 8193)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, files, err := prepareSamples(root, Options{Samples: []Sample{{Input: source, Answer: source}}}); err != nil || len(files) != 0 {
		t.Fatal("large sample staged")
	}
}
