package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectoryDigestProtectsManifestSemantics(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "artifact.json"), []byte("policy=exact"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("13:artifact.json:12:policy=exact"))
	expected := hex.EncodeToString(sum[:])
	if err := VerifyDirectory(directory, expected); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "artifact.json"), []byte("policy=tokens"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDirectory(directory, expected); err == nil {
		t.Fatal("changed comparison manifest retained release identity")
	}
}

func TestArtifactReferenceRejectsSymlinkedParents(t *testing.T) {
	directory, outside := t.TempDir(), t.TempDir()
	data := []byte("secret")
	if err := os.WriteFile(filepath.Join(outside, "data"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	sum := sha256.Sum256(data)
	if _, err := Verify(directory, "link/data", BlobRef{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}); err == nil {
		t.Fatal("read escaped artifact root")
	}
}

func TestPublishedSchemaRemainsReadableAfterCheckPolicyUpgrade(t *testing.T) {
	directory := t.TempDir()
	manifest := ArtifactManifest{SchemaVersion: 1, ToolchainKey: strings.Repeat("a", 64), Snapshot: FrozenSnapshot{SchemaVersion: 1, PolicyVersion: "vertex-authoring-3", TreeHash: strings.Repeat("b", 64), Tests: []FrozenTest{{ID: "one"}}}, Tests: []ArtifactTest{{}}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "artifact.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(directory); err != nil {
		t.Fatalf("policy upgrade invalidated pinned release: %v", err)
	}
	manifest.SchemaVersion = 99
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(directory, "artifact.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(directory); err == nil {
		t.Fatal("unknown executable schema accepted")
	}
}
