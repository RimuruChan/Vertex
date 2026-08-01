package store

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// 测试数据文件名模式:NN.in / NN.out(支持 NN 任意位数字,不含路径)。
var (
	testdataInRe  = regexp.MustCompile(`^(\d+)\.in$`)
	testdataOutRe = regexp.MustCompile(`^(\d+)\.out$`)
)

// extractTestdataZip 解压测试数据 zip 到 dir,按编号配对 in/out。
// 校验:1..N 必须完整(允许从 1 开始不连续?——严格要求连续 1..N)。
// 返回 case 数。
func extractTestdataZip(zipData []byte, dir string) (int, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return 0, fmt.Errorf("open zip: %w", err)
	}

	inputs := map[int]string{}  // caseNo -> zip 内条目名
	outputs := map[int]string{}
	maxNo := 0

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if m := testdataInRe.FindStringSubmatch(name); m != nil {
			n := atoiOr0(m[1])
			inputs[n] = f.Name
			if n > maxNo {
				maxNo = n
			}
			continue
		}
		if m := testdataOutRe.FindStringSubmatch(name); m != nil {
			n := atoiOr0(m[1])
			outputs[n] = f.Name
		}
	}

	// 校验连续性 1..maxNo
	var count int
	for i := 1; i <= maxNo; i++ {
		inName, okIn := inputs[i]
		outName, okOut := outputs[i]
		if !okIn || !okOut {
			return 0, fmt.Errorf("testdata incomplete: case %d missing %s%s", i, ternary(okIn, "", "input "), ternary(okOut, "", "output "))
		}
		if err := extractFile(zr, inName, filepath.Join(dir, fmt.Sprintf("%d.in", i))); err != nil {
			return 0, err
		}
		if err := extractFile(zr, outName, filepath.Join(dir, fmt.Sprintf("%d.out", i))); err != nil {
			return 0, err
		}
		count++
	}

	if count == 0 {
		return 0, fmt.Errorf("no testdata found in zip (expected 1.in/1.out ...)")
	}
	return count, nil
}

func extractFile(zr *zip.Reader, entryName, destPath string) error {
	f, err := zr.Open(entryName)
	if err != nil {
		return err
	}
	defer f.Close()

	// 上限防护:单文件 32MB,防 zip 炸弹
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, io.LimitReader(f, 32*1024*1024)); err != nil {
		return err
	}
	return nil
}

func atoiOr0(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
