package authoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var artifactHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Clone copies regular files, never links, into a new problem's private tree.
// Callers hold source authorization until the destination transaction commits.
func (p *TestdataPublisher) Clone(ctx context.Context, sourceID, targetID string, source PackageUpload) (_ *PackageUpload, err error) {
	if !artifactScopeRe.MatchString(sourceID) || !artifactScopeRe.MatchString(targetID) || sourceID == targetID ||
		!artifactHashPattern.MatchString(source.SHA256) || source.StoragePath != path.Join(sourceID, source.SHA256) {
		return nil, ErrPackageTarget
	}
	if source.CaseCount <= 0 || source.CaseCount > maxPackageEntries/2 {
		return nil, ErrPackageTooBig
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	// Reject links at each stored path component as well as at each file.
	for _, name := range []string{sourceID, source.StoragePath} {
		info, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrPackageTarget
		}
	}
	from, err := root.OpenRoot(source.StoragePath)
	if err != nil {
		return nil, err
	}
	defer from.Close()
	directory, err := from.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(maxPackageEntries + 1)
	directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if len(entries) > maxPackageEntries {
		return nil, ErrPackageTooBig
	}
	expected := make(map[string]bool, source.CaseCount*2+1)
	for index := 1; index <= source.CaseCount; index++ {
		expected[strconv.Itoa(index)+".in"] = true
		expected[strconv.Itoa(index)+".out"] = true
	}
	if source.Checker == "testlib" {
		expected[CheckerFileName] = true
	}
	if len(entries) != len(expected) {
		return nil, ErrPackageTarget
	}
	for _, entry := range entries {
		if !expected[entry.Name()] || !entry.Type().IsRegular() {
			return nil, ErrPackageTarget
		}
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	// A copy always gets a fresh ID. Never reuse or remove an existing target.
	if err := root.Mkdir(targetID, 0755); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, root.RemoveAll(targetID))
		}
	}()
	storagePath := path.Join(targetID, source.SHA256)
	if err := root.Mkdir(storagePath, 0755); err != nil {
		return nil, err
	}
	to, err := root.OpenRoot(storagePath)
	if err != nil {
		return nil, err
	}
	defer to.Close()
	framed, legacy := sha256.New(), sha256.New()
	remaining := maxPackageUncompressedBytes
	for _, entry := range entries {
		name := entry.Name()
		info, err := from.Lstat(name)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, ErrPackageTarget
		}
		if info.Size() > maxPackageFileBytes || info.Size() > remaining {
			return nil, ErrPackageTooBig
		}
		fmt.Fprintf(framed, "%d:%s:%d:", len(name), name, info.Size())
		_, _ = legacy.Write([]byte(name))
		if err := cloneArtifactFile(ctx, from, to, name, info, remaining, io.MultiWriter(framed, legacy)); err != nil {
			return nil, err
		}
		remaining -= info.Size()
	}
	// Existing manual imports use name+bytes; built packages use framed names
	// and lengths. Preserve their hash, validating either producer's format.
	if hex.EncodeToString(framed.Sum(nil)) != source.SHA256 && hex.EncodeToString(legacy.Sum(nil)) != source.SHA256 {
		return nil, ErrPackageTarget
	}
	result := source
	result.StoragePath, result.created = storagePath, true
	return &result, nil
}

func cloneArtifactFile(ctx context.Context, from, to *os.Root, name string, expected os.FileInfo, remaining int64, digest io.Writer) error {
	input, err := from.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	actual, err := input.Stat()
	if err != nil {
		return err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		return ErrPackageTarget
	}
	output, err := to.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	limit := min(maxPackageFileBytes, remaining)
	written, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(copyReader{ctx, input}, limit+1))
	closeErr := output.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if written != expected.Size() || written > limit {
		return ErrPackageTarget
	}
	return ctx.Err()
}

type copyReader struct {
	ctx    context.Context
	source io.Reader
}

func (r copyReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(data)
}
