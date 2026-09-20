package run

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ValidateInputPath is for trusted staging into a descriptor-rooted workspace.
// Stdin and artifact export keep their stricter one-component contracts.
func ValidateInputPath(name string) error {
	if name == "" || len(name) > 1024 || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") || strings.Count(name, "/") > 31 {
		return fmt.Errorf("invalid sandbox input path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." || len(part) > 255 {
			return fmt.Errorf("invalid sandbox input path %q", name)
		}
	}
	return nil
}

func copyInputs(ctx context.Context, directory string, files map[string]string, limit int64) error {
	if limit <= 0 || limit == math.MaxInt64 {
		return fmt.Errorf("invalid workspace byte limit")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		if err := ValidateInputPath(name); err != nil {
			return err
		}
		names = append(names, name)
	}
	sort.Strings(names)
	remaining := limit
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0755); err != nil {
			return fmt.Errorf("create input directory: %w", err)
		}
		if err := root.Remove(filepath.FromSlash(name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale input: %w", err)
		}
		if err := copyRootedInput(root, filepath.FromSlash(name), files[name], &remaining); err != nil {
			return err
		}
	}
	return nil
}

func copyRootedInput(root *os.Root, name, source string, remaining *int64) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > *remaining {
		return fmt.Errorf("input exceeds workspace limit or is not a regular file")
	}
	dst, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("create sandbox input: %w", err)
	}
	written, copyErr := io.Copy(dst, io.LimitReader(src, *remaining+1))
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written > *remaining {
		return fmt.Errorf("input exceeds remaining workspace budget")
	}
	*remaining -= written
	return nil
}

func chmodInput(directory, name string) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("executable input is not a regular file")
	}
	return file.Chmod(0755)
}
