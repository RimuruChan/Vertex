package filesystem

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (p *TestdataPublisher) CloneChecked(ctx context.Context, sourceID, targetID string, source domain.PackageUpload, snapshot domain.CheckSnapshot) (*domain.PackageUpload, error) {
	if sourceID == targetID || !artifactScopeRe.MatchString(sourceID) || !artifactScopeRe.MatchString(targetID) || !artifactHashPattern.MatchString(source.SHA256) || source.StoragePath != path.Join(sourceID, source.SHA256) || source.Artifact == nil {
		return nil, domain.ErrPackageTarget
	}
	manifest := *source.Artifact
	fingerprint, err := snapshot.DataFingerprint()
	if err != nil || fingerprint != snapshot.DataHash || fingerprint != manifest.Snapshot.DataHash || snapshot.PolicyVersion != domain.CheckPolicyVersion || source.CaseCount != len(manifest.Tests) || source.CaseCount < 1 || source.CaseCount > 10000 {
		return nil, domain.ErrPackageTarget
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, name := range []string{sourceID, source.StoragePath} {
		info, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, domain.ErrPackageTarget
		}
	}
	from, err := root.OpenRoot(source.StoragePath)
	if err != nil {
		return nil, err
	}
	defer from.Close()
	// Verify the stored descriptor too; source.Artifact came from the immutable
	// release binding and is not authority to bypass on-disk integrity checks.
	data, err := p.Read(ctx, sourceID, source.StoragePath, "artifact.json", 8<<20)
	if err != nil {
		return nil, err
	}
	var actual domain.CheckArtifact
	if err := json.Unmarshal(data, &actual); err != nil {
		return nil, err
	}
	expectedJSON, _ := json.Marshal(manifest)
	actualJSON, _ := json.Marshal(actual)
	if !bytes.Equal(expectedJSON, actualJSON) {
		return nil, domain.ErrPackageTarget
	}
	names := []string{}
	err = fs.WalkDir(from.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return domain.ErrPackageTarget
		}
		if err := domain.ValidatePackagePath(name); err != nil {
			return err
		}
		names = append(names, name)
		if len(names) > 60002 {
			return domain.ErrPackageTooBig
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	staging, err := os.MkdirTemp(p.root, ".checked-copy-"+targetID+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)
	to, err := os.OpenRoot(staging)
	if err != nil {
		return nil, err
	}
	digest := sha256.New()
	remaining := MaxPackageBytes
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(staging, filepath.FromSlash(path.Dir(name))), 0755); err != nil {
			to.Close()
			return nil, err
		}
		info, err := from.Lstat(name)
		if err != nil {
			to.Close()
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxPackageFileBytes || info.Size() > remaining {
			to.Close()
			return nil, domain.ErrPackageTarget
		}
		fmt.Fprintf(digest, "%d:%s:%d:", len(name), name, info.Size())
		if err := cloneArtifactFile(ctx, from, to, name, info, remaining, digest); err != nil {
			to.Close()
			return nil, err
		}
		remaining -= info.Size()
	}
	if hex.EncodeToString(digest.Sum(nil)) != source.SHA256 {
		to.Close()
		return nil, domain.ErrPackageTarget
	}
	manifest.Snapshot = snapshot
	encoded, err := json.Marshal(manifest)
	if err != nil {
		to.Close()
		return nil, err
	}
	if len(encoded) > 8<<20 || int64(len(encoded)-len(data)) > remaining {
		to.Close()
		return nil, domain.ErrPackageTooBig
	}
	file, err := to.OpenFile("artifact.json", os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		to.Close()
		return nil, err
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	to.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	nextHash, err := directoryDigest(staging)
	if err != nil {
		return nil, err
	}
	storagePath, target, err := p.artifactTarget(targetID, nextHash)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return nil, err
	}
	// Destination identity is new and private until the database commits.
	if err := os.Rename(staging, target); err != nil {
		return nil, err
	}
	return &domain.PackageUpload{Artifact: &manifest, StoragePath: storagePath, SHA256: nextHash, CaseCount: len(manifest.Tests), Checker: snapshot.Metadata.Comparison.Kind, Created: true}, nil
}
