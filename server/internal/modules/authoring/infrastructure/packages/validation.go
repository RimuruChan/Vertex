package packages

import (
	"fmt"
	"path"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"go.yaml.in/yaml/v3"
)

func validationMode(name string) string {
	for _, mode := range []string{"invalid_input", "invalid_output", "valid_output"} {
		if strings.HasPrefix(name, "data/"+mode+"/") {
			return mode
		}
	}
	return ""
}

func (value *importer) validationCases(version string) error {
	count := 0
	for _, name := range value.archive.names {
		mode := validationMode(name)
		if mode == "" {
			continue
		}
		if version != "2025-09" {
			value.require("kattis.validation_version", name, "校验器自测目录属于 2025-09 格式；请确认题包格式后再执行", "build")
		}
		if !strings.HasSuffix(name, ".in") {
			continue
		}
		count++
		if count > domain.MaxValidationCases {
			return invalid("校验器自测最多 1000 项")
		}
		base := strings.TrimSuffix(name, ".in")
		input, err := value.file(name, domain.EntryInput, nil)
		if err != nil {
			return err
		}
		definition := domain.ValidationMaterial{SchemaVersion: 1, Name: strings.TrimPrefix(base, "data/"), Mode: mode, Input: input.ID}
		if mode == "invalid_input" {
			if value.archive.has(base+".ans") || value.archive.has(base+".out") {
				return invalid("无效输入自测不能包含答案或输出：%s", base)
			}
		} else {
			for _, suffix := range []string{".ans", ".out"} {
				if !value.archive.has(base + suffix) {
					return invalid("输出自测缺少 %s：%s", suffix, base)
				}
				file, err := value.file(base+suffix, domain.EntryAnswer, nil)
				if err != nil {
					return err
				}
				if suffix == ".ans" {
					definition.Answer = file.ID
				} else {
					definition.Output = file.ID
				}
			}
		}
		// Configuration is inherited from the root through each directory. Only
		// descriptions are interpreted here; unsupported execution flags remain
		// explicit build blockers instead of silently changing validator behavior.
		configs := []string{base + ".yaml"}
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			configs = append([]string{dir + "/test_group.yaml"}, configs...)
		}
		for _, configPath := range configs {
			if !value.archive.has(configPath) {
				continue
			}
			data, err := value.archive.read(configPath, 1<<20)
			if err != nil {
				return err
			}
			config, err := readYAML(data)
			if err != nil {
				return err
			}
			if description, ok := config["description"]; ok {
				text, ok := description.(string)
				if !ok {
					return invalid("自测 description 必须是字符串")
				}
				definition.Description = text
			}
		}
		id := stableID("validation", base)
		if err := value.document(id, "vertex/validation/"+id+".json", domain.EntryValidation, definition); err != nil {
			return err
		}
	}
	for _, name := range value.archive.names {
		if validationMode(name) == "" {
			continue
		}
		if strings.HasSuffix(name, ".ans") || strings.HasSuffix(name, ".out") {
			if !value.archive.has(strings.TrimSuffix(name, path.Ext(name)) + ".in") {
				return invalid("自测文件没有对应输入：%s", name)
			}
		}
	}
	return nil
}

func (value *importer) validationConfiguration(name string) (bool, error) {
	if validationMode(name) == "" {
		return false, nil
	}
	data, err := value.archive.read(name, 1<<20)
	if err != nil {
		return false, err
	}
	config, err := readYAML(data)
	if err != nil {
		return false, err
	}
	for key, raw := range config {
		if key == "description" {
			if _, ok := raw.(string); ok {
				continue
			}
		}
		return false, nil
	}
	return true, nil
}

func exportValidation(tree domain.ContentTree, version string, files map[string]exportFile, read func(domain.BlobRef) ([]byte, error)) error {
	entries := map[string]domain.TreeEntry{}
	for _, entry := range tree.Entries {
		entries[entry.ID] = entry
	}
	index := 0
	for _, entry := range tree.Entries {
		if entry.Kind != domain.EntryValidation {
			continue
		}
		if version != "2025-09" {
			return invalid("校验器自测需导出为 Kattis 2025-09 或 Vertex 原生归档")
		}
		data, err := read(entry.Blob)
		if err != nil {
			return err
		}
		view, err := domain.DecodeMaterial(entry, data)
		if err != nil {
			return err
		}
		item := view.Validation
		index++
		base := fmt.Sprintf("data/%s/%04d", item.Mode, index)
		for _, reference := range []struct{ id, suffix, kind string }{{item.Input, ".in", domain.EntryInput}, {item.Answer, ".ans", domain.EntryAnswer}, {item.Output, ".out", domain.EntryAnswer}} {
			if reference.id == "" {
				continue
			}
			source, ok := entries[reference.id]
			if !ok || source.Kind != reference.kind {
				return invalid("自测文件引用无效")
			}
			ref := source.Blob
			files[base+reference.suffix] = exportFile{ref: &ref}
		}
		if item.Description != "" {
			body, err := yaml.Marshal(map[string]string{"description": item.Description})
			if err != nil {
				return err
			}
			files[base+".yaml"] = exportFile{body: body}
		}
	}
	return nil
}
