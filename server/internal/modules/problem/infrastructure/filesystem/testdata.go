package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
)

type TestdataStorage struct{ root string }

// A resource ID is one opaque directory name, never a relative path.
var problemIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func validateProblemID(id string) error {
	if !problemIDPattern.MatchString(id) {
		return fmt.Errorf("invalid problem ID")
	}
	return nil
}

func NewTestdataStorage(root string) *TestdataStorage { return &TestdataStorage{root: root} }
func (s *TestdataStorage) Materialize(problemID string, archive []byte) (problemdomain.TestdataArtifact, error) {
	count, hash, path, err := materializeTestdata(s.root, problemID, archive)
	return problemdomain.TestdataArtifact{CaseCount: count, SHA256: hash, StoragePath: path}, err
}
func (s *TestdataStorage) RemoveProblem(problemID string) error {
	if err := validateProblemID(problemID); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(problemID)
}

var _ problemdomain.ArtifactStorage = (*TestdataStorage)(nil)

func materializeTestdata(root, problemID string, zipData []byte) (int, string, string, error) {
	if err := validateProblemID(problemID); err != nil {
		return 0, "", "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, "", "", err
	}
	storage, err := os.OpenRoot(root)
	if err != nil {
		return 0, "", "", err
	}
	defer storage.Close()
	staging, err := os.MkdirTemp(root, ".testdata-upload-")
	if err != nil {
		return 0, "", "", err
	}
	defer storage.RemoveAll(filepath.Base(staging))

	count, err := extractTestdataZip(zipData, staging)
	if err != nil {
		return 0, "", "", err
	}
	hash, err := dirSHA256(staging)
	if err != nil {
		return 0, "", "", err
	}
	hashText := hex.EncodeToString(hash[:])
	storagePath := path.Join(problemID, hashText)
	target := filepath.FromSlash(storagePath)
	if err := storage.MkdirAll(problemID, 0o755); err != nil {
		return 0, "", "", err
	}
	if _, err := storage.Stat(target); errors.Is(err, os.ErrNotExist) {
		if renameErr := storage.Rename(filepath.Base(staging), target); renameErr != nil {
			// An identical concurrent upload may have won the rename race.
			if _, statErr := storage.Stat(target); statErr != nil {
				return 0, "", "", renameErr
			}
		}
	} else if err != nil {
		return 0, "", "", err
	}
	return count, hashText, storagePath, nil
}

func dirSHA256(dir string) ([32]byte, error) {
	var h [32]byte
	entries, err := os.ReadDir(dir)
	if err != nil {
		return h, err
	}
	hasher := sha256.New()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return h, err
		}
		hasher.Write([]byte(e.Name()))
		hasher.Write(data)
	}
	copy(h[:], hasher.Sum(nil))
	return h, nil
}
