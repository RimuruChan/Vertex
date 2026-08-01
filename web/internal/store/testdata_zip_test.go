package store

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func makeTestdataZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestExtractTestdataZip(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid zip", func(t *testing.T) {
		zipData := makeTestdataZip(t, map[string]string{
			"1.in":  "1\n",
			"1.out": "1\n",
			"2.in":  "2\n",
			"2.out": "2\n",
		})
		count, err := extractTestdataZip(zipData, dir)
		if err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Errorf("count = %d, want 2", count)
		}
		// 验证文件存在且内容正确
		data, err := os.ReadFile(filepath.Join(dir, "2.in"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "2\n" {
			t.Errorf("2.in content = %q", string(data))
		}
	})

	t.Run("subdirectories tolerated", func(t *testing.T) {
		subDir := filepath.Join(dir, "sub")
		mkdir(t, subDir)
		zipData := makeTestdataZip(t, map[string]string{
			"data/1.in":  "a\n",
			"data/1.out": "a\n",
		})
		count, err := extractTestdataZip(zipData, subDir)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("count = %d, want 1", count)
		}
	})

	t.Run("missing pair errors", func(t *testing.T) {
		badDir := filepath.Join(dir, "bad")
		mkdir(t, badDir)
		zipData := makeTestdataZip(t, map[string]string{
			"1.in":  "x\n",
			"1.out": "y\n",
			"2.in":  "z\n", // 缺 2.out
		})
		if _, err := extractTestdataZip(zipData, badDir); err == nil {
			t.Error("expected error for missing 2.out")
		}
	})

	t.Run("empty zip errors", func(t *testing.T) {
		emptyDir := filepath.Join(dir, "empty")
		mkdir(t, emptyDir)
		if _, err := extractTestdataZip(makeTestdataZip(t, map[string]string{}), emptyDir); err == nil {
			t.Error("expected error for empty zip")
		}
	})

	t.Run("not a zip errors", func(t *testing.T) {
		notZipDir := filepath.Join(dir, "notzip")
		mkdir(t, notZipDir)
		if _, err := extractTestdataZip([]byte("not a zip"), notZipDir); err == nil {
			t.Error("expected error for non-zip data")
		}
	})
}
