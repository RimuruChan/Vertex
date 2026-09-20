package packages

import (
	"bytes"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

func readYAML(data []byte) (map[string]any, error) {
	if len(data) > 1<<20 {
		return nil, invalid("YAML 配置超过 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		return nil, invalid("无效 YAML：%s", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, invalid("配置必须只包含一份 YAML 文档")
	}
	remaining := 100000
	var inspect func(*yaml.Node, int, map[*yaml.Node]bool) error
	inspect = func(node *yaml.Node, depth int, ancestors map[*yaml.Node]bool) error {
		remaining--
		if remaining < 0 || depth > 64 || ancestors[node] {
			return invalid("YAML 结构或别名展开过于复杂")
		}
		ancestors[node] = true
		defer delete(ancestors, node)
		if node.Kind == yaml.AliasNode {
			return inspect(node.Alias, depth+1, ancestors)
		}
		if node.Kind == yaml.MappingNode {
			keys := map[string]bool{}
			for i := 0; i < len(node.Content); i += 2 {
				key := node.Content[i]
				if key.Kind != yaml.ScalarNode || (key.Tag != "!!str" && !(key.Value == "<<" && key.Tag == "!!merge")) {
					return invalid("YAML 键必须为字符串")
				}
				if keys[key.Value] {
					return invalid("YAML 键重复：%s", key.Value)
				}
				keys[key.Value] = true
			}
		}
		for _, child := range node.Content {
			if err := inspect(child, depth+1, ancestors); err != nil {
				return err
			}
		}
		return nil
	}
	if err := inspect(&root, 0, map[*yaml.Node]bool{}); err != nil {
		return nil, err
	}
	var result map[string]any
	if err := root.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	if result == nil {
		return nil, invalid("YAML 配置必须为映射")
	}
	return result, nil
}
