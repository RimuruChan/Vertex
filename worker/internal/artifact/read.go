package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/RimuruChan/Vertex/worker/internal/run"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// VerifyDirectory binds the manifest as well as all files to the immutable
// digest selected by the server. A manifest cannot change comparator semantics
// while retaining otherwise valid per-file hashes.
func VerifyDirectory(directory, expected string) error {
	if !digestPattern.MatchString(expected) {
		return fmt.Errorf("invalid artifact digest")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	names := []string{}
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact contains a non-regular entry")
		}
		names = append(names, name)
		if len(names) > 60002 {
			return fmt.Errorf("artifact has too many files")
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(names)
	hash := sha256.New()
	remaining := int64(64 << 20)
	for _, name := range names {
		file, err := root.Open(name)
		if err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > remaining {
			file.Close()
			return fmt.Errorf("artifact exceeds size limit")
		}
		fmt.Fprintf(hash, "%d:%s:%d:", len(name), name, info.Size())
		size, err := io.Copy(hash, io.LimitReader(file, remaining+1))
		file.Close()
		if err != nil {
			return err
		}
		if size > remaining {
			return fmt.Errorf("artifact grew beyond its limit")
		}
		remaining -= size
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("artifact differs from the selected release")
	}
	return nil
}

func Read(directory string) (*ArtifactManifest, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open("artifact.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("artifact manifest exceeds size limit")
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	// Published artifacts are pinned by digest and executable schema. Raising the
	// requirements for new checks must not invalidate an existing contest release.
	if manifest.SchemaVersion != 1 || manifest.Snapshot.SchemaVersion != 1 || manifest.Snapshot.PolicyVersion == "" || !digestPattern.MatchString(manifest.ToolchainKey) || !digestPattern.MatchString(manifest.Snapshot.TreeHash) || len(manifest.Tests) == 0 || len(manifest.Tests) != len(manifest.Snapshot.Tests) {
		return nil, fmt.Errorf("invalid artifact identity or test set")
	}
	return &manifest, nil
}

// Verify confines reads to the artifact root and checks exact bytes before they
// enter a compiler or judging environment. Inputs are immutable once published.
func Verify(directory, name string, ref BlobRef) (string, error) {
	if err := run.ValidateInputPath(name); err != nil {
		return "", err
	}
	if !digestPattern.MatchString(ref.SHA256) || ref.Bytes < 0 || ref.Bytes > 64<<20 {
		return "", fmt.Errorf("invalid artifact reference")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(filepath.FromSlash(name))
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("artifact entry is not a regular file")
	}
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || opened.Size() != ref.Bytes || !os.SameFile(info, opened) {
		return "", fmt.Errorf("artifact file changed or has wrong size")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, ref.Bytes+1))
	if err != nil {
		return "", err
	}
	if size != ref.Bytes || hex.EncodeToString(hash.Sum(nil)) != ref.SHA256 {
		return "", fmt.Errorf("artifact content hash mismatch")
	}
	return filepath.Join(directory, filepath.FromSlash(name)), nil
}
