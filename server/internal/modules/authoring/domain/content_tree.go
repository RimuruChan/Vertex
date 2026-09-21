package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// ContentTree is an immutable manifest, not the contents of the package. Large
// test data, statements and sources all live in blobs. Entry IDs survive moves;
// paths are names in the package, never paths on the server's filesystem.
type ContentTree struct {
	Entries []TreeEntry `json:"entries"`
}

type BlobRef struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type TreeEntry struct {
	ID         string            `json:"id"`
	Path       string            `json:"path"`
	Kind       string            `json:"kind"`
	Blob       BlobRef           `json:"blob"`
	Attributes map[string]string `json:"attributes"`
}

const (
	EntryMetadata  = "metadata"
	EntryStatement = "statement"
	EntrySource    = "source"
	EntryTest      = "test"
	EntryGroup     = "group"
	EntryInput     = "input"
	EntryAnswer    = "answer"
	EntryAsset     = "asset"
	EntryResource  = "resource"
	MaxTreeEntries = 10000
)

var (
	contentIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func Reference(data []byte) BlobRef {
	return BlobRef{SHA256: Digest(data), Bytes: int64(len(data))}
}

// Canonical validates and copies the tree. The caller cannot mutate the returned
// manifest by changing input maps or slices after hashing or saving it.
func (tree ContentTree) Canonical() (ContentTree, error) {
	if len(tree.Entries) > MaxTreeEntries {
		return ContentTree{}, InvalidInput("too many package entries")
	}
	result := ContentTree{Entries: make([]TreeEntry, 0, len(tree.Entries))}
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, entry := range tree.Entries {
		if !contentIDPattern.MatchString(entry.ID) || ids[entry.ID] {
			return ContentTree{}, InvalidInput("invalid or duplicate entry ID: " + entry.ID)
		}
		ids[entry.ID] = true
		if err := ValidatePackagePath(entry.Path); err != nil {
			return ContentTree{}, err
		}
		folded := strings.ToLower(entry.Path)
		if paths[folded] {
			return ContentTree{}, InvalidInput("duplicate package path: " + entry.Path)
		}
		paths[folded] = true
		switch entry.Kind {
		case EntryMetadata, EntryProgram, EntryStatement, EntrySource, EntryTest, EntryGroup, EntryValidation, EntryGeneration, EntryInput, EntryAnswer, EntryAsset, EntryResource:
		default:
			return ContentTree{}, InvalidInput("unknown package entry kind: " + entry.Kind)
		}
		if !digestPattern.MatchString(entry.Blob.SHA256) || entry.Blob.Bytes < 0 {
			return ContentTree{}, InvalidInput("invalid blob reference: " + entry.Path)
		}
		if len(entry.Attributes) > 32 {
			return ContentTree{}, InvalidInput("too many entry attributes: " + entry.Path)
		}
		copy := cloneEntry(entry)
		for key, value := range copy.Attributes {
			if !contentIDPattern.MatchString(key) || !utf8.ValidString(value) || len(value) > 4096 {
				return ContentTree{}, InvalidInput("invalid entry attribute: " + entry.Path)
			}
		}
		result.Entries = append(result.Entries, copy)
	}
	for name := range paths {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if paths[parent] {
				return ContentTree{}, InvalidInput("package file shadows a directory: " + parent)
			}
		}
	}
	slices.SortFunc(result.Entries, func(a, b TreeEntry) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}

func (tree ContentTree) Encode() ([]byte, error) {
	canonical, err := tree.Canonical()
	if err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func (tree ContentTree) Hash() (string, error) {
	data, err := tree.Encode()
	if err != nil {
		return "", err
	}
	return Digest(data), nil
}

// ValidatePackagePath uses portable relative paths even on Windows. The blob
// store itself uses only hashes, but archives and build workspaces need the same
// path contract to prevent ambiguous extraction on another operating system.
func ValidatePackagePath(name string) error {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || path.Clean(name) != name ||
		strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") {
		return InvalidInput("invalid package path: " + name)
	}
	parts := strings.Split(name, "/")
	if len(parts) > 32 {
		return InvalidInput("package path is too deep: " + name)
	}
	for _, part := range parts {
		if part == "." || part == ".." || len(part) > 255 || strings.TrimRight(part, " .") != part {
			return InvalidInput("invalid package path component: " + name)
		}
		for _, ch := range part {
			if ch < 32 || ch == 127 || strings.ContainsRune("<>\"|?*", ch) {
				return InvalidInput("invalid package path character: " + name)
			}
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
			return InvalidInput("reserved package path: " + name)
		}
		if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) &&
			stem[3] >= '0' && stem[3] <= '9' {
			return InvalidInput("reserved package path: " + name)
		}
		if stem == "COM¹" || stem == "COM²" || stem == "COM³" || stem == "LPT¹" || stem == "LPT²" || stem == "LPT³" {
			return InvalidInput("reserved package path: " + name)
		}
	}
	return nil
}

func cloneEntry(entry TreeEntry) TreeEntry {
	copy := entry
	copy.Attributes = make(map[string]string, len(entry.Attributes))
	for key, value := range entry.Attributes {
		copy.Attributes[key] = value
	}
	return copy
}

func ValidateBlob(ref BlobRef, data []byte) error {
	if ref != Reference(data) {
		return fmt.Errorf("blob content does not match reference %s", ref.SHA256)
	}
	return nil
}
