package checker

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/vertex-oj/judge/internal/verdict"
)

// Checker 判定 stdout 与期望输出是否一致。
// MVP 只支持内置 checker:标准 diff(忽略行尾空白与末尾空行)。
// v1 在此扩展 SPJ(调用外部 checker 二进制)。

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
	return io.ReadAll(io.LimitReader(f, 16*1024*1024)) // 输出上限已在沙箱侧控制
}
