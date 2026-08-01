package api

import "os"

// testdataRoot 返回测试数据卷根目录(环境变量 TESTDATA_ROOT,默认 ./testdata)。
func testdataRoot() string {
	if v := os.Getenv("TESTDATA_ROOT"); v != "" {
		return v
	}
	return "./testdata"
}
