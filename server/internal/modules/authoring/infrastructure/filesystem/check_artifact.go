package filesystem

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func extractCheckArtifact(archive []byte, directory string) (*domain.CheckArtifact, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, domain.InvalidInput("invalid build archive")
	}
	var descriptor *zip.File
	for _, file := range reader.File {
		if file.Name == "artifact.json" {
			if descriptor != nil {
				return nil, domain.InvalidInput("duplicate artifact manifest")
			}
			descriptor = file
		}
	}
	if descriptor == nil {
		return nil, nil
	}
	if len(reader.File) > 60002 || descriptor.UncompressedSize64 > 8<<20 {
		return nil, domain.ErrPackageTooBig
	}
	stream, err := descriptor.Open()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(stream, (8<<20)+1))
	stream.Close()
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, domain.ErrPackageTooBig
	}
	var manifest domain.CheckArtifact
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !json.Valid(data) || decoder.Decode(&manifest) != nil {
		return nil, domain.InvalidInput("invalid artifact manifest")
	}
	if manifest.SchemaVersion != 1 || manifest.Snapshot.SchemaVersion != 1 || manifest.Snapshot.PolicyVersion != domain.CheckPolicyVersion || !blobDigestPattern.MatchString(manifest.ToolchainKey) || !blobDigestPattern.MatchString(manifest.Snapshot.TreeHash) {
		return nil, domain.InvalidInput("unsupported artifact version or identity")
	}
	fingerprint, err := manifest.Snapshot.DataFingerprint()
	if err != nil || fingerprint != manifest.Snapshot.DataHash {
		return nil, domain.InvalidInput("artifact data fingerprint mismatch")
	}
	if len(manifest.Tests) == 0 || len(manifest.Tests) != len(manifest.Snapshot.Tests) || len(manifest.Tests) > 10000 {
		return nil, domain.InvalidInput("artifact test set does not match snapshot")
	}
	wanted := map[string]*domain.BlobRef{"artifact.json": nil}
	add := func(name string, ref domain.BlobRef) error {
		if err := domain.ValidatePackagePath(name); err != nil {
			return err
		}
		if _, exists := wanted[name]; exists {
			return domain.InvalidInput("duplicate artifact path: " + name)
		}
		if !blobDigestPattern.MatchString(ref.SHA256) || ref.Bytes < 0 || ref.Bytes > maxPackageFileBytes {
			return domain.InvalidInput("invalid artifact file reference")
		}
		wanted[name] = &ref
		return nil
	}
	for index, test := range manifest.Tests {
		if test.ID != manifest.Snapshot.Tests[index].ID {
			return nil, domain.InvalidInput("artifact test order mismatch")
		}
		if err := add(fmt.Sprintf("%d.in", index+1), test.Input); err != nil {
			return nil, err
		}
		if err := add(fmt.Sprintf("%d.out", index+1), test.Answer); err != nil {
			return nil, err
		}
	}
	for _, program := range manifest.Snapshot.Programs {
		for _, file := range program.Files {
			if err := add("programs/"+program.ID+"/"+file.Path, file.Blob); err != nil {
				return nil, err
			}
		}
	}
	for _, dependency := range manifest.Dependencies {
		if dependency.ID != "testlib" || dependency.Path != "testlib.h" {
			return nil, domain.InvalidInput("unknown artifact dependency")
		}
		if err := add("dependencies/"+dependency.Path, dependency.Blob); err != nil {
			return nil, err
		}
	}
	if len(manifest.Statements) != len(manifest.Snapshot.Statements) || len(manifest.Statements) > 20 {
		return nil, domain.InvalidInput("artifact statements do not match snapshot")
	}
	for index, statement := range manifest.Statements {
		if statement.ID != manifest.Snapshot.Statements[index].ID || statement.PDF.Bytes > 32<<20 {
			return nil, domain.InvalidInput("invalid rendered statement binding")
		}
		if err := add("statements/"+statement.ID+".pdf", statement.PDF); err != nil {
			return nil, err
		}
	}
	seen := map[string]bool{}
	remaining := MaxPackageBytes
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if !file.Mode().IsRegular() {
			return nil, domain.InvalidInput("artifact entries must be regular files")
		}
		if err := domain.ValidatePackagePath(file.Name); err != nil {
			return nil, err
		}
		fold := strings.ToLower(file.Name)
		if seen[fold] {
			return nil, domain.InvalidInput("duplicate artifact entry")
		}
		seen[fold] = true
		ref, exists := wanted[file.Name]
		if !exists {
			return nil, domain.InvalidInput("undeclared artifact file: " + file.Name)
		}
		target := filepath.Join(directory, filepath.FromSlash(file.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		if err := writeEntry(file, target, &remaining); err != nil {
			return nil, err
		}
		if ref != nil {
			content, err := os.Open(target)
			if err != nil {
				return nil, err
			}
			actual, err := hashArtifactFile(content)
			content.Close()
			if err != nil {
				return nil, err
			}
			if actual != *ref {
				return nil, domain.InvalidInput("artifact file digest mismatch: " + file.Name)
			}
		}
	}
	for name := range wanted {
		if !seen[strings.ToLower(name)] {
			return nil, domain.InvalidInput("artifact file is missing: " + name)
		}
	}
	return &manifest, nil
}

func hashArtifactFile(file io.Reader) (domain.BlobRef, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	return domain.BlobRef{SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: size}, err
}
