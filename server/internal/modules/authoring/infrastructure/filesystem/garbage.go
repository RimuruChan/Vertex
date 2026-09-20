package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

const namespaceUUID = `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`

var namespacePattern = regexp.MustCompile(`^` + namespaceUUID + `$`)
var stagingPattern = regexp.MustCompile(`^\.(?:package-upload|checked-copy|testdata-upload)-(` + namespaceUUID + `)-[0-9]+$`)
var trashPattern = regexp.MustCompile(`^\.gc-[0-9a-f]{64}-[0-9a-f]{32}$`)
var blobTemporaryPattern = regexp.MustCompile(`^\.upload-[0-9]+$`)

type GarbageStorage struct{ blobs, artifacts string }

func NewGarbageStorage(blobs *BlobStore, artifacts *TestdataPublisher) *GarbageStorage {
	return &GarbageStorage{blobs: blobs.root, artifacts: artifacts.root}
}

func (storage *GarbageStorage) Namespaces(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	for _, name := range []string{storage.blobs, storage.artifacts} {
		root, err := os.OpenRoot(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		entries, err := fs.ReadDir(root.FS(), ".")
		root.Close()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if namespacePattern.MatchString(entry.Name()) {
				seen[entry.Name()] = true
			}
			if match := stagingPattern.FindStringSubmatch(entry.Name()); match != nil {
				seen[match[1]] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

// The caller must hold the database storage exclusion lock. Database reference
// removal commits first; deleting these unreferenced files can safely be retried.
func (storage *GarbageStorage) Sweep(ctx context.Context, id string, refs domain.StorageReferences, cutoff time.Time) (domain.SweepResult, error) {
	result := domain.SweepResult{}
	if !namespacePattern.MatchString(id) {
		return result, domain.InvalidInput("invalid collection namespace")
	}
	for _, bucket := range []struct {
		root      string
		artifacts bool
	}{{storage.blobs, false}, {storage.artifacts, true}} {
		root, err := os.OpenRoot(bucket.root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		err = func() error {
			defer root.Close()
			info, err := root.Lstat(id)
			if err == nil {
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return domain.ErrPackageTarget
				}
				entries, err := fs.ReadDir(root.FS(), id)
				if err != nil {
					return err
				}
				for _, entry := range entries {
					if err := ctx.Err(); err != nil {
						return err
					}
					if result.Objects >= 256 {
						break
					}
					name := entry.Name()
					target := path.Join(id, name)
					if entry.Type()&os.ModeSymlink != 0 {
						continue
					}
					// A tombstone is never a live path. Renaming before recursive
					// deletion prevents a crash leaving a partial canonical artifact.
					if trashPattern.MatchString(name) {
						size, err := storedSize(ctx, root, target)
						if err != nil {
							return err
						}
						if err := root.RemoveAll(target); err != nil {
							return err
						}
						result.Objects++
						result.Bytes += size
						continue
					}
					isBlobTemp := !bucket.artifacts && blobTemporaryPattern.MatchString(name) && entry.Type().IsRegular()
					if !isBlobTemp && !blobDigestPattern.MatchString(name) {
						continue
					}
					if bucket.artifacts && !entry.IsDir() || !bucket.artifacts && !entry.Type().IsRegular() {
						continue
					}
					if !isBlobTemp && ((bucket.artifacts && refs.Artifacts[target]) || (!bucket.artifacts && refs.Blobs[name])) {
						continue
					}
					info, err := root.Lstat(target)
					if err != nil {
						return err
					}
					if !info.ModTime().Before(cutoff) {
						continue
					}
					size, err := storedSize(ctx, root, target)
					if err != nil {
						return err
					}
					if isBlobTemp {
						if err := root.Remove(target); err != nil {
							return err
						}
					} else {
						var nonce [16]byte
						if _, err := rand.Read(nonce[:]); err != nil {
							return err
						}
						trash := path.Join(id, ".gc-"+name+"-"+hex.EncodeToString(nonce[:]))
						if err := root.Rename(target, trash); err != nil {
							return err
						}
						if err := ctx.Err(); err != nil {
							return err
						}
						if err := root.RemoveAll(trash); err != nil {
							return err
						}
					}
					result.Objects++
					result.Bytes += size
				}
				// Remove only an empty namespace, never unknown operator files.
				remaining, err := fs.ReadDir(root.FS(), id)
				if err == nil && len(remaining) == 0 {
					_ = root.Remove(id)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if bucket.artifacts && result.Objects < 256 {
				entries, err := fs.ReadDir(root.FS(), ".")
				if err != nil {
					return err
				}
				for _, entry := range entries {
					match := stagingPattern.FindStringSubmatch(entry.Name())
					if match == nil || match[1] != id || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
						continue
					}
					if result.Objects >= 256 {
						break
					}
					info, err := root.Lstat(entry.Name())
					if err != nil {
						return err
					}
					if !info.ModTime().Before(cutoff) {
						continue
					}
					size, err := storedSize(ctx, root, entry.Name())
					if err != nil {
						return err
					}
					if err := root.RemoveAll(entry.Name()); err != nil {
						return err
					}
					result.Objects++
					result.Bytes += size
				}
			}
			return nil
		}()
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func storedSize(ctx context.Context, root *os.Root, name string) (int64, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return 0, err
	}
	if info.Mode().IsRegular() {
		return info.Size(), nil
	}
	if !info.IsDir() {
		return 0, domain.ErrPackageTarget
	}
	var total int64
	err = fs.WalkDir(root.FS(), name, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if current != name && !strings.HasPrefix(current, name+"/") {
			return domain.ErrPackageTarget
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return domain.ErrPackageTarget
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
		return err
	})
	return total, err
}
