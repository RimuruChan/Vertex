package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

var blobDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// OpenPublished uses a descriptor already selected by the authorized release
// reader. The storage layer never turns a caller-supplied path into a file.
func (store *BlobStore) OpenPublished(ctx context.Context, problemID, digest string, size int64) (io.ReadCloser, error) {
	return store.Open(ctx, problemID, domain.BlobRef{SHA256: digest, Bytes: size})
}

// BlobStore stores immutable package material independently of judge artifacts.
// Objects are streamed to a staging file and atomically linked into place. An
// existing content-addressed file is verified and never overwritten/deleted.
type BlobStore struct {
	root     string
	maxBytes int64
}

func NewBlobStore(root string, maxBytes int64) (*BlobStore, error) {
	if root == "" || maxBytes <= 0 || maxBytes == int64(^uint64(0)>>1) {
		return nil, errors.New("blob store requires a root and a positive bounded size limit")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &BlobStore{root: absolute, maxBytes: maxBytes}, nil
}

func (store *BlobStore) directory(problemID string) (string, error) {
	if !artifactScopeRe.MatchString(problemID) {
		return "", domain.InvalidInput("invalid blob namespace")
	}
	return filepath.Join(store.root, problemID), nil
}

func (store *BlobStore) Put(ctx context.Context, problemID string, reader io.Reader) (domain.BlobRef, error) {
	directory, err := store.directory(problemID)
	if err != nil {
		return domain.BlobRef{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.BlobRef{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return domain.BlobRef{}, err
	}
	temporary, err := os.CreateTemp(directory, ".upload-")
	if err != nil {
		return domain.BlobRef{}, err
	}
	defer func() { _ = temporary.Close(); _ = os.Remove(temporary.Name()) }()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(contextReader{ctx, reader}, store.maxBytes+1))
	if err != nil {
		return domain.BlobRef{}, err
	}
	if written > store.maxBytes {
		return domain.BlobRef{}, domain.ErrPackageTooBig
	}
	if err := temporary.Sync(); err != nil {
		return domain.BlobRef{}, err
	}
	if err := temporary.Close(); err != nil {
		return domain.BlobRef{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.BlobRef{}, err
	}
	ref := domain.BlobRef{SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: written}
	target := filepath.Join(directory, ref.SHA256)
	if err := os.Link(temporary.Name(), target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return domain.BlobRef{}, err
		}
		file, err := store.Open(ctx, problemID, ref)
		if err != nil {
			return domain.BlobRef{}, err
		}
		defer file.Close()
		existingHash := sha256.New()
		if _, err := io.Copy(existingHash, contextReader{ctx, file}); err != nil {
			return domain.BlobRef{}, err
		}
		if hex.EncodeToString(existingHash.Sum(nil)) != ref.SHA256 {
			return domain.BlobRef{}, errors.New("existing blob is corrupt")
		}
	}
	return ref, nil
}

func (store *BlobStore) Open(ctx context.Context, problemID string, ref domain.BlobRef) (io.ReadCloser, error) {
	directory, err := store.directory(problemID)
	if err != nil {
		return nil, err
	}
	if !blobDigestPattern.MatchString(ref.SHA256) || ref.Bytes < 0 || ref.Bytes > store.maxBytes {
		return nil, domain.InvalidInput("invalid blob reference")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := filepath.Join(directory, ref.SHA256)
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != ref.Bytes {
		return nil, fmt.Errorf("blob %s has invalid type or size", ref.SHA256)
	}
	return os.Open(name)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(data)
}
