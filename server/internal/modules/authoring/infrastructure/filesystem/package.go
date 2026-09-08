package filesystem

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Build artifact layout. The judge side already resolves testdata as
// <root>/<problemId>/<contentHash>/, so a built package reuses that layout and
// simply adds the checker source next to the data it was validated against.
const (
	CheckerFileName = "checker.cpp"
	// MaxPackageBytes bounds a single uploaded artifact. It matches the manual
	// testdata upload limit so both paths hit the same operational ceiling.
	MaxPackageBytes = int64(64 << 20)
	// maxPackageFileBytes bounds one entry inside the artifact.
	maxPackageFileBytes = int64(64 << 20)
	// maxPackageUncompressedBytes bounds the aggregate extracted snapshot. A
	// small, highly compressed archive must not exhaust the shared testdata
	// volume by spreading data across many individually valid files.
	maxPackageUncompressedBytes = MaxPackageBytes
	// maxPackageEntries stops an archive with an implausible number of files
	// before any of them is written to disk.
	maxPackageEntries = 4096
)

var (
	packageInputRe  = regexp.MustCompile(`^(\d{1,6})\.in$`)
	packageAnswerRe = regexp.MustCompile(`^(\d{1,6})\.out$`)
	artifactScopeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

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
	staging, err := os.MkdirTemp(p.root, ".package-upload-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	count, hasChecker, err := extractPackage(archive, staging)
	if err != nil {
		return nil, err
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

	checker := "diff"
	if hasChecker {
		checker = "testlib"
	}
	return &authoringdomain.PackageUpload{
		StoragePath: storagePath, SHA256: digest, CaseCount: count, Checker: checker,
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

// extractPackage writes the archive into dir after checking that it contains a
// contiguous 1..N test set and nothing but recognized entries. Rejecting
// unknown names keeps a compromised or buggy worker from planting files the
// judge side would later execute.
func extractPackage(archive []byte, dir string) (int, bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return 0, false, authoringdomain.InvalidInput("build package is not a readable zip archive")
	}
	if len(reader.File) > maxPackageEntries {
		return 0, false, authoringdomain.InvalidInput("build package contains too many entries")
	}

	inputs := map[int]*zip.File{}
	answers := map[int]*zip.File{}
	var checker *zip.File
	highest := 0
	var declaredBytes uint64
	declaredLimit := uint64(maxPackageUncompressedBytes)

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := path.Base(entry.Name)
		if name != entry.Name {
			return 0, false, authoringdomain.InvalidInput("build package entries must not be nested: " + entry.Name)
		}
		if entry.UncompressedSize64 > uint64(maxPackageFileBytes) {
			return 0, false, authoringdomain.InvalidInput("build package entry exceeds the per-file limit: " + name)
		}
		if entry.UncompressedSize64 > declaredLimit-declaredBytes {
			return 0, false, authoringdomain.InvalidInput("build package exceeds the aggregate uncompressed size limit")
		}
		declaredBytes += entry.UncompressedSize64
		switch {
		case packageInputRe.MatchString(name):
			index := mustIndex(packageInputRe, name)
			inputs[index] = entry
			if index > highest {
				highest = index
			}
		case packageAnswerRe.MatchString(name):
			answers[mustIndex(packageAnswerRe, name)] = entry
		case name == CheckerFileName:
			checker = entry
		default:
			return 0, false, authoringdomain.InvalidInput("unexpected build package entry: " + name)
		}
	}

	if highest == 0 {
		return 0, false, authoringdomain.InvalidInput("build package contains no tests")
	}
	remainingBytes := maxPackageUncompressedBytes
	for index := 1; index <= highest; index++ {
		input, hasInput := inputs[index]
		answer, hasAnswer := answers[index]
		if !hasInput || !hasAnswer {
			return 0, false, authoringdomain.InvalidInput(fmt.Sprintf("build package is missing test %d", index))
		}
		if err := writeEntry(input, filepath.Join(dir, strconv.Itoa(index)+".in"), &remainingBytes); err != nil {
			return 0, false, err
		}
		if err := writeEntry(answer, filepath.Join(dir, strconv.Itoa(index)+".out"), &remainingBytes); err != nil {
			return 0, false, err
		}
	}
	if len(inputs) != highest || len(answers) != highest {
		return 0, false, authoringdomain.InvalidInput("build package test indexes must be contiguous from 1")
	}
	if checker != nil {
		if err := writeEntry(checker, filepath.Join(dir, CheckerFileName), &remainingBytes); err != nil {
			return 0, false, err
		}
	}
	return highest, checker != nil, nil
}

func mustIndex(pattern *regexp.Regexp, name string) int {
	matches := pattern.FindStringSubmatch(name)
	index, _ := strconv.Atoi(matches[1])
	return index
}

func writeEntry(entry *zip.File, destination string, remainingBytes *int64) error {
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := copyPackageEntry(source, file, entry.Name, remainingBytes); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
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
