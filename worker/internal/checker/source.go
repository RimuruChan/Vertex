package checker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/RimuruChan/Vertex/worker/internal/compile"
)

// SourceCompiler builds the checker that ships inside a testdata snapshot.
//
// The snapshot carries the checker as source rather than as a binary: the
// judge node compiles it with its own toolchain, so a package built on one
// machine cannot smuggle a foreign executable onto another. The compile cache
// makes this a one-time cost per checker revision.
type SourceCompiler struct {
	compiler      *compile.Compiler
	testlibPath   string
	testlibDigest string
}

func NewSourceCompiler(compiler *compile.Compiler, testlibPath string) (*SourceCompiler, error) {
	if compiler == nil {
		return nil, fmt.Errorf("checker source compiler requires a compiler")
	}
	header, err := os.ReadFile(testlibPath)
	if err != nil {
		return nil, fmt.Errorf("read testlib header %s: %w", testlibPath, err)
	}
	digest := sha256.Sum256(header)
	return &SourceCompiler{
		compiler:      compiler,
		testlibPath:   testlibPath,
		testlibDigest: hex.EncodeToString(digest[:]),
	}, nil
}

// TestlibDigest identifies the header this node compiles checkers against.
func (s *SourceCompiler) TestlibDigest() string { return s.testlibDigest }

// Compile returns the host-side path of the compiled checker binary.
func (s *SourceCompiler) Compile(ctx context.Context, source []byte) (string, error) {
	digest := sha256.Sum256(source)
	path, result := s.compiler.CompileExt(ctx, "cpp", source, hex.EncodeToString(digest[:]),
		TestlibExtension(s.testlibPath, s.testlibDigest))
	if !result.OK {
		return "", fmt.Errorf("compile checker: %s", result.Error)
	}
	return path, nil
}
