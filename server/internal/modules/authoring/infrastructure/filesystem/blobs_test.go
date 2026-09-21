package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestBlobStoreConcurrentImmutableWrites(t *testing.T) {
	store, err := NewBlobStore(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 8 {
		wait.Go(func() {
			ref, err := store.Put(context.Background(), "problem", strings.NewReader("original"))
			if err != nil || ref != domain.Reference([]byte("original")) {
				t.Errorf("put = %+v %v", ref, err)
			}
		})
	}
	wait.Wait()
	ref := domain.Reference([]byte("original"))
	file, err := store.Open(context.Background(), "problem", ref)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil || string(data) != "original" {
		t.Fatalf("read = %q %v", data, err)
	}
	if _, err := store.Open(context.Background(), "other", ref); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blob escaped namespace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.root, "problem", ref.SHA256), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "problem", strings.NewReader("original")); err == nil {
		t.Fatal("corrupt existing blob accepted")
	}
}

func TestBlobStoreLimitsAndCancellation(t *testing.T) {
	store, err := NewBlobStore(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "problem", strings.NewReader("12345")); !errors.Is(err, domain.ErrPackageTooBig) {
		t.Fatalf("size limit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(ctx, "problem", strings.NewReader("123")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	for _, name := range []string{"../problem", "/tmp", "C:/data", `a\b`} {
		if _, err := store.Put(context.Background(), name, strings.NewReader("a")); err == nil {
			t.Fatalf("unsafe namespace %q", name)
		}
	}
	entries, err := os.ReadDir(filepath.Join(store.root, "problem"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed upload left files: %v %v", entries, err)
	}
}
