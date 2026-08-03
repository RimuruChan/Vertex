package checker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// MaxOutputBytes is shared with the executor so the checker never compares a
// truncated prefix of an otherwise valid sandbox output.
const MaxOutputBytes int64 = 32 * 1024 * 1024

// Checker 判定 stdout 与期望输出是否一致。
// Checker performs the built-in normalized diff. It never executes an
// untrusted external checker binary.

// CheckDiff 比较实际输出文件与期望输出文件。
// 返回判定 + checker 说明。文件不存在按 WA/SE 处理。
func CheckDiff(actualPath, expectedPath string) (string, string, error) {
	actual, err := readFile(actualPath)
	if err != nil {
		return verdict.SE, "cannot read program output: " + err.Error(), nil
	}
	expected, err := readFile(expectedPath)
	if err != nil {
		return verdict.SE, "cannot read expected output: " + err.Error(), nil
	}

	if outputsEqual(actual, expected) {
		return verdict.AC, "ok", nil
	}
	return verdict.WA, "output mismatch", nil
}

// Normalize 去掉行尾空白与文件末尾空行,作为判定前归一化。
func Normalize(b []byte) []byte {
	lines := strings.Split(string(b), "\n")
	// 去尾部空行
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return []byte(strings.Join(lines, "\n"))
}

func outputsEqual(a, b []byte) bool {
	return bytes.Equal(Normalize(a), Normalize(b))
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxOutputBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxOutputBytes {
		return nil, fmt.Errorf("file exceeds checker limit of %d bytes", MaxOutputBytes)
	}
	return data, nil
}
