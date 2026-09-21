package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestGarbageStoragePreservesReferencesAndUnknownFiles(t *testing.T) {
	ctx := context.Background()
	id := "01234567-89ab-4cde-8f01-234567890abc"
	blobs, err := NewBlobStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	publisher := NewTestdataPublisher(t.TempDir())
	storage := NewGarbageStorage(blobs, publisher)
	live, err := blobs.Put(ctx, id, strings.NewReader("live"))
	if err != nil {
		t.Fatal(err)
	}
	dead, err := blobs.Put(ctx, id, strings.NewReader("dead"))
	if err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(blobs.root, id, "operator-note.txt")
	if err := os.WriteFile(unknown, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	artifact := strings.Repeat("a", 64)
	artifactPath := filepath.Join(publisher.root, id, artifact)
	if err := os.MkdirAll(artifactPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactPath, "data"), []byte("still live"), 0600); err != nil {
		t.Fatal(err)
	}
	trash := filepath.Join(publisher.root, id, ".gc-"+artifact+"-"+strings.Repeat("b", 32))
	if err := os.MkdirAll(trash, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trash, "partial"), []byte("abandoned sweep"), 0600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(publisher.root, ".package-upload-"+id+"-123456")
	if err := os.MkdirAll(staging, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := storage.Sweep(ctx, id, domain.StorageReferences{Blobs: map[string]bool{live.SHA256: true}, Artifacts: map[string]bool{id + "/" + artifact: true}}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Objects != 3 {
		t.Fatalf("unexpected collected objects: %+v", result)
	}
	for _, name := range []string{filepath.Join(blobs.root, id, live.SHA256), unknown, artifactPath} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("removed retained object %s: %v", name, err)
		}
	}
	for _, name := range []string{filepath.Join(blobs.root, id, dead.SHA256), trash, staging} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("garbage remains %s: %v", name, err)
		}
	}
	if _, err := storage.Sweep(ctx, "../escape", domain.StorageReferences{}, time.Now()); err == nil {
		t.Fatal("unsafe namespace accepted")
	}
}

func TestGarbageStorageDoesNotFollowLinks(t *testing.T) {
	id := "12345678-9abc-4def-8012-34567890abcd"
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	blobs, err := NewBlobStore(root, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, id), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, id, strings.Repeat("c", 64))
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	storage := NewGarbageStorage(blobs, NewTestdataPublisher(t.TempDir()))
	if _, err := storage.Sweep(context.Background(), id, domain.StorageReferences{}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatal("collector traversed external link")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("collector touched an unexpected symbolic link")
	}
}
