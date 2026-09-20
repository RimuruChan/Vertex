package packages

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"path"
	"sort"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Same commit and digest as worker/Dockerfile. License is retained in the header.
const exportTestlibSHA256 = "bb323e3c89285214966076e0d23d5a295c5f6126da7ff198c1276ddb95ecb1a0"

//go:embed adapters/testlib.h
var exportTestlib []byte

//go:embed adapters/run.py
var testlibAdapter string

//go:embed adapters/exact.cpp
var exactValidator []byte

type adapterConfig struct {
	SchemaVersion int      `json:"schemaVersion"`
	Role          string   `json:"role"`
	EntryPoint    string   `json:"entryPoint"`
	Files         []string `json:"files"`
	Arguments     []string `json:"arguments"`
}

// Recognize only our exact generated build/run contract. Arbitrary package
// scripts remain unsupported; this path reconstructs data, never executes them.
func (value *importer) importTestlibAdapter(group, role string) (string, bool, error) {
	marker := group + "/vertex-adapter.json"
	if !value.archive.has(marker) {
		return "", false, nil
	}
	data, err := value.archive.read(marker, 1<<20)
	if err != nil {
		return "", true, err
	}
	var config adapterConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !json.Valid(data) || decoder.Decode(&config) != nil || config.SchemaVersion != 1 || config.Role != role || (role != "input-validator" && role != "output-validator") || len(config.Files) == 0 || len(config.Files) > 1000 || config.Arguments == nil {
		return "", true, invalid("无效的 testlib 适配器描述")
	}
	seen := map[string]bool{}
	allowed := map[string]bool{"vertex-adapter.json": true, "build": true, "run": true, "VERTEX-README": true, "testlib.h": true}
	for _, name := range config.Files {
		if domain.ValidatePackagePath(name) != nil || seen[strings.ToLower(name)] {
			return "", true, invalid("testlib 适配器源文件路径无效")
		}
		seen[strings.ToLower(name)] = true
		allowed[name] = true
		allowed[path.Join(path.Dir(name), "testlib.h")] = true
		if !value.archive.has(group + "/" + name) {
			return "", true, invalid("testlib 适配器源文件缺失")
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".cpp", ".cc", ".cxx", ".c", ".h", ".hpp", ".hh", ".hxx":
		default:
			return "", true, invalid("testlib 适配器源文件类型无效")
		}
	}
	if !seen[strings.ToLower(config.EntryPoint)] {
		return "", true, invalid("testlib 适配器入口缺失")
	}
	build, err := adapterBuild(config)
	if err != nil {
		return "", true, err
	}
	for name, expected := range map[string]string{"build": build, "run": testlibAdapter} {
		actual, err := value.archive.read(group+"/"+name, 1<<20)
		if err != nil || string(actual) != expected {
			return "", true, invalid("testlib 适配脚本已修改，不能按已知协议自动导入")
		}
	}
	for _, name := range value.archive.names {
		if !strings.HasPrefix(name, group+"/") {
			continue
		}
		relative := strings.TrimPrefix(name, group+"/")
		if !allowed[relative] {
			return "", true, invalid("testlib 适配器包含未声明的伴随文件：%s", relative)
		}
	}
	for name := range allowed {
		if path.Base(name) != "testlib.h" {
			continue
		}
		header, err := value.archive.read(group+"/"+name, 1<<20)
		if err != nil || domain.Digest(header) != exportTestlibSHA256 {
			return "", true, invalid("testlib 适配器使用了不同的头文件")
		}
	}
	id := stableID("program", group)
	definition := domain.ProgramMaterial{SchemaVersion: 1, Name: path.Base(group), Directory: group, Role: role, Language: "cpp", Protocol: "testlib", Files: []string{}, Arguments: config.Arguments}
	for _, name := range config.Files {
		entry, err := value.file(group+"/"+name, domain.EntrySource, map[string]string{"language": "cpp", "originFormat": value.plan.Format})
		if err != nil {
			return "", true, err
		}
		definition.Files = append(definition.Files, entry.ID)
		if name == config.EntryPoint {
			definition.EntryPoint = entry.ID
		}
	}
	if definition.EntryPoint == "" {
		return "", true, invalid("testlib 适配器入口大小写不匹配")
	}
	if err := value.document(id, "vertex/programs/"+id+".json", domain.EntryProgram, definition); err != nil {
		return "", true, err
	}
	if role == "output-validator" {
		value.metadata.Comparison = domain.OutputComparison{Kind: "testlib"}
	}
	return id, true, nil
}

func adapterBuild(config adapterConfig) (string, error) {
	units := []string{}
	for _, name := range config.Files {
		switch strings.ToLower(path.Ext(name)) {
		case ".c", ".cpp", ".cc", ".cxx":
			units = append(units, name)
		}
	}
	if len(units) == 0 {
		return "", invalid("校验器缺少编译单元")
	}
	sort.Strings(units)
	command := "#!/bin/sh\nset -eu\nexec c++ -std=c++17 -O2 -I."
	for _, name := range units {
		command += " '" + strings.ReplaceAll("./"+name, "'", "'\"'\"'") + "'"
	}
	return command + " -o vertex_original\n", nil
}

func adaptTestlib(files map[string]exportFile, prefix string, program domain.ProgramMaterial, entries map[string]domain.TreeEntry) error {
	if program.Language != "cpp" {
		return invalid("testlib 标准导出适配目前要求 C++ 程序")
	}
	config := adapterConfig{SchemaVersion: 1, Role: program.Role, Files: []string{}, Arguments: append([]string{}, program.Arguments...)}
	headerDirs := map[string]bool{prefix: true}
	for _, id := range program.Files {
		source, ok := entries[id]
		if !ok {
			return invalid("校验器材料缺失")
		}
		name := source.Path
		if program.Directory != "" {
			name = strings.TrimPrefix(name, program.Directory+"/")
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hh", ".hxx":
		default:
			return invalid("testlib 运行时伴随文件需要额外映射：%s", name)
		}
		config.Files = append(config.Files, name)
		if id == program.EntryPoint {
			config.EntryPoint = name
		}
		headerDirs[path.Dir(prefix+"/"+name)] = true
	}
	if config.EntryPoint == "" {
		return invalid("testlib 入口不存在")
	}
	for directory := range headerDirs {
		name := directory + "/testlib.h"
		if prior, exists := files[name]; exists && (prior.ref == nil || prior.ref.SHA256 != exportTestlibSHA256) {
			return invalid("包内 testlib.h 与已配置工具链不同")
		}
		files[name] = exportFile{body: exportTestlib}
	}
	sort.Strings(config.Files)
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	build, err := adapterBuild(config)
	if err != nil {
		return err
	}
	for _, name := range []string{"build", "run", "vertex_original", "vertex-adapter.json", "VERTEX-README"} {
		if _, exists := files[prefix+"/"+name]; exists {
			return invalid("校验器适配路径冲突：%s", name)
		}
	}
	files[prefix+"/build"] = exportFile{body: []byte(build), mode: 0755}
	files[prefix+"/run"] = exportFile{body: []byte(testlibAdapter), mode: 0755}
	files[prefix+"/vertex-adapter.json"] = exportFile{body: append(encoded, '\n')}
	files[prefix+"/VERTEX-README"] = exportFile{body: []byte("Requires POSIX, C++17 and Python 3. build compiles the unchanged original sources; run translates Kattis stdin/feedback and testlib exit codes. testlib.h is MIT licensed, pinned to " + exportTestlibSHA256 + ". Partial scores and checker failures are judge errors.\n")}
	return nil
}
