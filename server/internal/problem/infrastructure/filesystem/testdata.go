package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"

	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
)

type TestdataStorage struct{ root string }

func NewTestdataStorage(root string) *TestdataStorage { return &TestdataStorage{root: root} }
func (s *TestdataStorage) Materialize(problemID string, archive []byte) (problemdomain.TestdataArtifact, error) {
	count, hash, path, err := materializeTestdata(s.root, problemID, archive)
	return problemdomain.TestdataArtifact{CaseCount: count, SHA256: hash, StoragePath: path}, err
}
func (s *TestdataStorage) RemoveProblem(problemID string) error {
	return os.RemoveAll(filepath.Join(s.root, problemID))
}

var _ problemdomain.ArtifactStorage = (*TestdataStorage)(nil)

func materializeTestdata(root, problemID string, zipData []byte) (int, string, string, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return 0, "", "", err
	}
	staging, err := os.MkdirTemp(root, ".testdata-upload-")
	if err != nil {
		return 0, "", "", err
	}
	defer os.RemoveAll(staging)

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
	target := filepath.Join(root, filepath.FromSlash(storagePath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, "", "", err
	}
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if renameErr := os.Rename(staging, target); renameErr != nil {
			// An identical concurrent upload may have won the rename race.
			if _, statErr := os.Stat(target); statErr != nil {
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
