package builder

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CheckerFileName is the name the checker source takes inside the published
// snapshot. Judging recompiles it from source so the binary is always built by
// the same toolchain that judges the submission.
const CheckerFileName = "checker.cpp"

// buildArchive packs the materialized tests, plus the checker source when the
// package has one, into the zip the server publishes as testdata.
func buildArchive(workspace string, testCount int, checkerSource *SourceFile) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	for index := 1; index <= testCount; index++ {
		for _, suffix := range []string{".in", ".out"} {
			name := fmt.Sprintf("%d%s", index, suffix)
			if err := addFile(writer, name, filepath.Join(workspace, name)); err != nil {
				return nil, err
			}
		}
	}
	if checkerSource != nil {
		entry, err := writer.Create(CheckerFileName)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write([]byte(checkerSource.SourceCode)); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func addFile(writer *zip.Writer, name, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer source.Close()
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	if _, err := io.Copy(entry, source); err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	return nil
}

var (
	generatorNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)
	generatorArgRe  = regexp.MustCompile(`^[-A-Za-z0-9_.,=:+/@\[\]]{1,64}$`)
)

// ParseGenerateCommand splits a generator line into argv. The worker mirrors
// the server's rules rather than trusting them, because this argv is handed
// straight to execve inside the sandbox.
func ParseGenerateCommand(command string) (string, []string, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("生成命令为空")
	}
	if len(fields) > 33 {
		return "", nil, fmt.Errorf("生成命令参数过多")
	}
	if !generatorNameRe.MatchString(fields[0]) {
		return "", nil, fmt.Errorf("生成器名称非法: %q", fields[0])
	}
	for _, argument := range fields[1:] {
		if !generatorArgRe.MatchString(argument) {
			return "", nil, fmt.Errorf("生成器参数含有不支持的字符: %q", argument)
		}
	}
	return fields[0], fields[1:], nil
}

// normalizeText converts a manual test to LF line endings and guarantees the
// trailing newline that most solutions and checkers expect.
func normalizeText(value string) []byte {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return []byte(normalized)
}

func copyFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destination, io.LimitReader(source, maxTestBytes)); err != nil {
		destination.Close()
		return err
	}
	return destination.Close()
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// readHead returns the first limit bytes of a file as text, trimmed at a rune
// boundary so the JSON report never carries a broken UTF-8 sequence.
func readHead(path string, limit int) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	buffer := make([]byte, limit)
	read, err := file.Read(buffer)
	if read <= 0 || (err != nil && read == 0) {
		return ""
	}
	text := string(trimPartialRune(buffer[:read]))
	if fileSize(path) > int64(read) {
		text += "\n…"
	}
	return text
}

// trimPartialRune drops a trailing byte sequence that would decode as an
// incomplete UTF-8 rune.
func trimPartialRune(data []byte) []byte {
	for i := len(data) - 1; i >= 0 && i > len(data)-4; i-- {
		if data[i]&0xC0 != 0x80 {
			if expectedRuneLength(data[i]) > len(data)-i {
				return data[:i]
			}
			break
		}
	}
	return data
}

func expectedRuneLength(lead byte) int {
	switch {
	case lead&0x80 == 0:
		return 1
	case lead&0xE0 == 0xC0:
		return 2
	case lead&0xF0 == 0xE0:
		return 3
	case lead&0xF8 == 0xF0:
		return 4
	default:
		return 1
	}
}
