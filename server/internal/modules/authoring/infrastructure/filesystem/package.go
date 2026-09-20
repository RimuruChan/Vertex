package filesystem

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Frozen artifact uploads are separate from external problem-package imports.
const (
	MaxPackageBytes     = int64(64 << 20)
	maxPackageFileBytes = int64(64 << 20)
)

var artifactScopeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// TestdataPublisher materializes build artifacts into the shared testdata
// volume. Directories are content-addressed and never mutated in place, so a
// judge job that already claimed an older snapshot keeps reading exactly the
// bytes it was dispatched with.
type TestdataPublisher struct{ root string }

func NewTestdataPublisher(root string) *TestdataPublisher {
	return &TestdataPublisher{root: root}
}

// Publish extracts and validates a build artifact, then installs it under its
// content hash. It returns the storage path relative to the testdata root.
func (p *TestdataPublisher) Publish(problemID string, archive []byte) (*authoringdomain.PackageUpload, error) {
	if !artifactScopeRe.MatchString(problemID) {
		return nil, authoringdomain.InvalidInput("problem ID is not a safe artifact path segment")
	}
	if int64(len(archive)) > MaxPackageBytes {
		return nil, authoringdomain.ErrPackageTooBig
	}
	if err := os.MkdirAll(p.root, 0o755); err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(p.root, ".package-upload-"+problemID+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	artifact, err := extractCheckArtifact(archive, staging)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, authoringdomain.InvalidInput("frozen build artifact manifest is required")
	}

	digest, err := directoryDigest(staging)
	if err != nil {
		return nil, err
	}
	storagePath, target, err := p.artifactTarget(problemID, digest)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	created := false
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if renameErr := os.Rename(staging, target); renameErr != nil {
			// An identical concurrent build may have won the rename race; the
			// destination is content-addressed, so reusing it is safe.
			if _, statErr := os.Stat(target); statErr != nil {
				return nil, renameErr
			}
		} else {
			created = true
		}
	} else if err != nil {
		return nil, err
	}

	return &authoringdomain.PackageUpload{
		Artifact:    artifact,
		StoragePath: storagePath, SHA256: digest, CaseCount: len(artifact.Tests), Checker: artifact.Snapshot.Metadata.Comparison.Kind,
		Created: created,
	}, nil
}

func (p *TestdataPublisher) artifactTarget(problemID, digest string) (string, string, error) {
	storagePath := path.Join(problemID, digest)
	root, err := filepath.Abs(p.root)
	if err != nil {
		return "", "", err
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(storagePath)))
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative != filepath.Join(problemID, digest) {
		return "", "", authoringdomain.InvalidInput("artifact path escapes the testdata root")
	}
	return storagePath, target, nil
}

// Remove deletes an artifact directory that no testdata row references.
func (p *TestdataPublisher) Remove(storagePath string) error {
	cleaned := filepath.Clean(filepath.FromSlash(storagePath))
	if cleaned == "" || cleaned == "." || filepath.IsAbs(cleaned) {
		return fmt.Errorf("refusing to remove suspicious artifact path %q", storagePath)
	}
	target := filepath.Join(p.root, cleaned)
	rootAbs, err := filepath.Abs(p.root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || relative == ".." || len(relative) > 2 && relative[:3] == ".."+string(filepath.Separator) {
		return fmt.Errorf("refusing to remove artifact outside the testdata root")
	}
	return os.RemoveAll(targetAbs)
}

func writeEntry(root *os.Root, entry *zip.File, name string, remainingBytes *int64) (authoringdomain.BlobRef, error) {
	source, err := entry.Open()
	if err != nil {
		return authoringdomain.BlobRef{}, err
	}
	defer source.Close()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return authoringdomain.BlobRef{}, err
	}
	// Hash exactly the bytes written through this handle, without reopening a
	// path that could have been replaced between extraction and verification.
	hash := sha256.New()
	before := *remainingBytes
	if err := copyPackageEntry(source, io.MultiWriter(file, hash), entry.Name, remainingBytes); err != nil {
		_ = file.Close()
		return authoringdomain.BlobRef{}, err
	}
	if err := file.Close(); err != nil {
		return authoringdomain.BlobRef{}, err
	}
	return authoringdomain.BlobRef{SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: before - *remainingBytes}, nil
}

func copyPackageEntry(source io.Reader, destination io.Writer, entryName string, remainingBytes *int64) error {
	limit := maxPackageFileBytes
	aggregateLimited := false
	if *remainingBytes < limit {
		limit = *remainingBytes
		aggregateLimited = true
	}
	written, err := io.Copy(destination, io.LimitReader(source, limit))
	if err != nil {
		return err
	}
	var probe [1]byte
	extra, probeErr := source.Read(probe[:])
	if probeErr != nil && !errors.Is(probeErr, io.EOF) {
		return probeErr
	}
	if extra != 0 {
		if aggregateLimited {
			return authoringdomain.InvalidInput("build package exceeds the aggregate uncompressed size limit")
		}
		return authoringdomain.InvalidInput("build package entry exceeds the per-file limit: " + entryName)
	}
	*remainingBytes -= written
	return nil
}

// directoryDigest hashes the artifact deterministically: names are sorted, and
// both name and content length are mixed in so no two different directories
// can collide by concatenation.
func directoryDigest(dir string) (string, error) {
	names := []string{}
	err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("artifact contains a non-regular file")
		}
		relative, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)

	hasher := sha256.New()
	for _, name := range names {
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return "", err
		}
		fmt.Fprintf(hasher, "%d:%s:%d:", len(name), name, info.Size())
		if _, err := io.Copy(hasher, file); err != nil {
			file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
