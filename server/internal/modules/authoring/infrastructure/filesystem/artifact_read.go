package filesystem

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (p *TestdataPublisher) Read(ctx context.Context, problemID, storagePath, name string, limit int64) ([]byte, error) {
	if limit <= 0 || limit > MaxPackageBytes || path.Dir(storagePath) != problemID || !blobDigestPattern.MatchString(path.Base(storagePath)) {
		return nil, domain.InvalidInput("invalid artifact read")
	}
	if err := domain.ValidatePackagePath(name); err != nil {
		return nil, err
	}
	_, target, err := p.artifactTarget(problemID, path.Base(storagePath))
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, domain.ErrPackageTooBig
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, domain.ErrPackageTooBig
	}
	return data, nil
}
