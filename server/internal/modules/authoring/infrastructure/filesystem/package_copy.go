package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"regexp"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

var artifactHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

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
		return authoringdomain.ErrPackageTarget
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
		return authoringdomain.ErrPackageTarget
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
