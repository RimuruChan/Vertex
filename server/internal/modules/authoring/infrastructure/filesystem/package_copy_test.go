package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func TestCheckedCopyRejectsUnsafeSources(t *testing.T) {
	for _, mode := range []string{"missing-manifest", "changed", "missing", "extra", "file-link", "path-link", "same-target", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			manifest, files := artifactFixture(t)
			root := t.TempDir()
			publisher := NewTestdataPublisher(root)
			source, err := publisher.Publish("source", artifactZip(t, manifest, files))
			if err != nil {
				t.Fatal(err)
			}
			sourceID, targetID := "source", "target"
			folder := filepath.Join(root, filepath.FromSlash(source.StoragePath))
			ctx := context.Background()
			switch mode {
			case "missing-manifest":
				source.Artifact = nil
			case "changed":
				err = os.WriteFile(filepath.Join(folder, "1.out"), []byte("changed"), 0644)
			case "missing":
				err = os.Remove(filepath.Join(folder, "1.out"))
			case "extra":
				err = os.WriteFile(filepath.Join(folder, "undeclared"), []byte("extra"), 0644)
			case "file-link":
				if err := os.Remove(filepath.Join(folder, "1.out")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("1.in", filepath.Join(folder, "1.out")); err != nil {
					t.Skip("symlinks unavailable: " + err.Error())
				}
			case "path-link":
				if err := os.Symlink(filepath.Join(root, "source"), filepath.Join(root, "alias")); err != nil {
					t.Skip("symlinks unavailable: " + err.Error())
				}
				sourceID = "alias"
				source.StoragePath = "alias/" + source.SHA256
			case "same-target":
				targetID = sourceID
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			copied, err := publisher.CloneChecked(ctx, sourceID, targetID, *source, manifest.Snapshot)
			if err == nil || copied != nil {
				t.Fatal("unsafe copy accepted")
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			if mode == "missing-manifest" && !errors.Is(err, domain.ErrPackageTarget) {
				t.Fatalf("missing manifest accepted: %v", err)
			}
			if mode != "same-target" {
				if _, err := os.Stat(filepath.Join(root, "target")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed copy left target: %v", err)
				}
			}
		})
	}
}

func TestCheckedCopyDoesNotOverwriteExistingArtifact(t *testing.T) {
	manifest, files := artifactFixture(t)
	root := t.TempDir()
	publisher := NewTestdataPublisher(root)
	archive := artifactZip(t, manifest, files)
	source, err := publisher.Publish("source", archive)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := publisher.Publish("target", archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.CloneChecked(context.Background(), "source", "target", *source, manifest.Snapshot); err == nil {
		t.Fatal("existing artifact overwritten")
	}
	data, err := publisher.Read(context.Background(), "target", existing.StoragePath, "1.out", 1024)
	if err != nil || string(data) != files["1.out"] {
		t.Fatalf("existing bytes changed: %q %v", data, err)
	}
}
