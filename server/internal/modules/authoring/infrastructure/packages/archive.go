package packages

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

const MaxArchiveBytes = int64(64 << 20)
const MaxExpandedBytes = int64(128 << 20)
const MaxEntryBytes = int64(64 << 20)

type archive struct {
	files  map[string]*zip.File
	names  []string
	prefix string
}

func readArchive(data []byte) (*archive, error) {
	if int64(len(data)) > MaxArchiveBytes {
		return nil, domain.ErrPackageTooBig
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, domain.InvalidInput("无法读取 ZIP 题包")
	}
	if len(reader.File) > 20000 {
		return nil, domain.InvalidInput("题包文件数量过多")
	}
	result := &archive{files: map[string]*zip.File{}}
	folded := map[string]bool{}
	var expanded uint64
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		if err := domain.ValidatePackagePath(name); err != nil {
			return nil, err
		}
		fold := strings.ToLower(name)
		if folded[fold] {
			return nil, domain.InvalidInput("题包存在重复或大小写冲突路径：" + name)
		}
		folded[fold] = true
		if file.FileInfo().IsDir() {
			continue
		}
		if !file.Mode().IsRegular() && file.Mode()&os.ModeSymlink == 0 {
			return nil, domain.InvalidInput("题包包含特殊文件：" + name)
		}
		if file.UncompressedSize64 > uint64(MaxEntryBytes) || expanded > uint64(MaxExpandedBytes)-file.UncompressedSize64 {
			return nil, domain.ErrPackageTooBig
		}
		expanded += file.UncompressedSize64
		result.files[name] = file
		result.names = append(result.names, name)
	}
	sort.Strings(result.names)
	for name := range result.files {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := result.files[parent]; exists {
				return nil, domain.InvalidInput("文件与目录重名：" + parent)
			}
		}
	}
	return result, nil
}

func (a *archive) selectRoot() error {
	markers := []string{"vertex-package.json", "problem.yaml", "problem.xml"}
	for _, name := range markers {
		if _, ok := a.files[name]; ok {
			return nil
		}
	}
	roots := map[string]bool{}
	for _, name := range a.names {
		for _, marker := range markers {
			if strings.HasSuffix(name, "/"+marker) {
				roots[strings.TrimSuffix(name, marker)] = true
			}
		}
	}
	var candidates []string
	for root := range roots {
		candidates = append(candidates, root)
	}
	sort.Slice(candidates, func(i, j int) bool { return len(candidates[i]) < len(candidates[j]) })
	if len(candidates) == 0 {
		return nil
	}
	root := candidates[0]
	for _, name := range a.names {
		if !strings.HasPrefix(name, root) && !strings.HasPrefix(name, "__MACOSX/") && path.Base(name) != ".DS_Store" {
			return domain.InvalidInput("题包包含多个根目录，请只导入一个题目")
		}
	}
	a.prefix = root
	files := map[string]*zip.File{}
	names := []string{}
	for _, name := range a.names {
		if strings.HasPrefix(name, root) {
			relative := strings.TrimPrefix(name, root)
			files[relative] = a.files[name]
			names = append(names, relative)
		}
	}
	a.files, a.names = files, names
	return nil
}

func (a *archive) resolve(name string) (*zip.File, error) {
	visited := map[string]bool{}
	for depth := 0; depth < 32; depth++ {
		if visited[name] {
			return nil, domain.InvalidInput("题包链接形成循环：" + name)
		}
		visited[name] = true
		file, ok := a.files[name]
		if !ok {
			return nil, domain.InvalidInput("题包文件不存在：" + name)
		}
		if file.Mode()&os.ModeSymlink == 0 {
			return file, nil
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		target, err := io.ReadAll(io.LimitReader(stream, 1025))
		stream.Close()
		if err != nil {
			return nil, err
		}
		if len(target) > 1024 || len(target) == 0 || path.IsAbs(string(target)) || strings.ContainsAny(string(target), "\\:\x00") {
			return nil, domain.InvalidInput("非法题包链接：" + name)
		}
		name = path.Clean(path.Join(path.Dir(name), string(target)))
		if err := domain.ValidatePackagePath(name); err != nil {
			return nil, err
		}
	}
	return nil, domain.InvalidInput("题包链接层级过深")
}

func (a *archive) read(name string, limit int64) ([]byte, error) {
	file, err := a.resolve(name)
	if err != nil {
		return nil, err
	}
	if file.UncompressedSize64 > uint64(limit) {
		return nil, domain.ErrPackageTooBig
	}
	stream, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, domain.ErrPackageTooBig
	}
	return data, nil
}

func (a *archive) open(name string) (io.ReadCloser, error) {
	file, err := a.resolve(name)
	if err != nil {
		return nil, err
	}
	return file.Open()
}
func (a *archive) has(name string) bool { _, ok := a.files[name]; return ok }
func (a *archive) marker() string {
	for _, name := range []string{"vertex-package.json", "problem.yaml", "problem.xml"} {
		if a.has(name) {
			return name
		}
	}
	return ""
}
func invalid(format string, args ...any) error {
	return domain.InvalidInput(fmt.Sprintf(format, args...))
}
