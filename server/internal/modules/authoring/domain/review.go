package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

type ReviewField struct {
	Key    string `json:"key"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type ReviewItem struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	Label     string        `json:"label"`
	Change    string        `json:"change"`
	EntryIDs  []string      `json:"entryIds"`
	Fields    []ReviewField `json:"fields"`
	Truncated bool          `json:"truncated"`
}
type reviewDocument struct {
	entry TreeEntry
	value map[string]any
}

// StructuredReview groups changed bytes by their authored owner. It is derived
// from the same two authorized immutable trees as DiffTrees, never live HEAD.
func StructuredReview(before, after ContentTree, load func(TreeEntry) ([]byte, error)) ([]ReviewItem, error) {
	changes, err := DiffTrees(before, after)
	if err != nil {
		return nil, err
	}
	docs := map[string]reviewDocument{}
	owners := map[string][]string{}
	all := map[string]TreeEntry{}
	labels := map[string]string{}
	cache := map[string][]byte{}
	budget := int64(32 << 20)
	read := func(e TreeEntry) ([]byte, error) {
		if b, ok := cache[e.Blob.SHA256]; ok {
			return b, nil
		}
		if e.Blob.Bytes > 1<<20 || e.Blob.Bytes > budget {
			return nil, nil
		}
		budget -= e.Blob.Bytes
		data, err := load(e)
		if err != nil {
			return nil, err
		}
		if err = ValidateBlob(e.Blob, data); err != nil {
			return nil, err
		}
		cache[e.Blob.SHA256] = data
		return data, nil
	}
	for _, tree := range []ContentTree{before, after} {
		for _, e := range tree.Entries {
			all[e.ID] = e
			label := e.Attributes["label"]
			if label == "" {
				parts := strings.Split(e.Path, "/")
				label = parts[len(parts)-1]
			}
			if e.Kind == EntryStatement {
				label = "题面 · " + e.Attributes["language"]
			}
			labels[e.ID] = label
			if e.Kind == EntryTest && e.Attributes["generationPlan"] != "" {
				owner := e.Attributes["generationPlan"]
				present := false
				for _, id := range owners[e.ID] {
					if id == owner {
						present = true
					}
				}
				if !present {
					owners[e.ID] = append(owners[e.ID], owner)
				}
			}
			switch e.Kind {
			case EntryMetadata, EntryProgram, EntryTest, EntryGroup, EntryGeneration, EntryValidation:
				data, err := read(e)
				if err != nil {
					return nil, err
				}
				var value map[string]any
				if json.Unmarshal(data, &value) != nil {
					continue
				}
				docs[e.ID] = reviewDocument{e, value}
				if name, ok := value["name"].(string); ok {
					labels[e.ID] = name
				}
				if e.Kind == EntryMetadata {
					labels[e.ID] = "题目与评测规则"
				}
				refs := []string{}
				if files, ok := value["files"].([]any); ok {
					for _, f := range files {
						if id, ok := f.(string); ok {
							refs = append(refs, id)
						}
					}
				}
				if e.Kind == EntryTest {
					for _, key := range []string{"input", "answer"} {
						if obj, ok := value[key].(map[string]any); ok {
							if ref, ok := obj["entry"].(string); ok && ref != "" {
								refs = append(refs, ref)
							}
						}
					}
				}
				if e.Kind == EntryValidation {
					for _, key := range []string{"input", "answer", "output"} {
						if ref, ok := value[key].(string); ok && ref != "" {
							refs = append(refs, ref)
						}
					}
				}
				for _, ref := range refs {
					found := false
					for _, id := range owners[ref] {
						if id == e.ID {
							found = true
						}
					}
					if !found {
						owners[ref] = append(owners[ref], e.ID)
					}
				}
			}
		}
	}
	result := []ReviewItem{}
	index := map[string]int{}
	fieldBudget := 2 << 20
	format := func(value any) string { return reviewValue(value, labels) }
	for _, change := range changes {
		e := change.After
		if e == nil {
			e = change.Before
		}
		targets := owners[e.ID]
		if len(targets) == 0 {
			targets = []string{e.ID}
		}
		for _, owner := range targets {
			position, ok := index[owner]
			if !ok {
				position = len(result)
				index[owner] = position
				entry := all[owner]
				result = append(result, ReviewItem{ID: owner, Kind: entry.Kind, Label: labels[owner], Change: "modified", EntryIDs: []string{}, Fields: []ReviewField{}})
			}
			item := &result[position]
			item.EntryIDs = append(item.EntryIDs, e.ID)
			if owner == e.ID {
				item.Change = change.Kind
			}
			if doc, ok := docs[e.ID]; ok && owner == e.ID {
				previous, next := map[string]any{}, map[string]any{}
				if change.Before != nil {
					data, err := read(*change.Before)
					if err != nil {
						return nil, err
					}
					_ = json.Unmarshal(data, &previous)
				}
				if change.After != nil {
					data, err := read(*change.After)
					if err != nil {
						return nil, err
					}
					_ = json.Unmarshal(data, &next)
				}
				_ = doc
				keys := map[string]bool{}
				for key := range previous {
					keys[key] = true
				}
				for key := range next {
					keys[key] = true
				}
				ordered := []string{}
				for key := range keys {
					ordered = append(ordered, key)
				}
				sort.Strings(ordered)
				for _, key := range ordered {
					if key == "schemaVersion" || key == "directory" || reflect.DeepEqual(previous[key], next[key]) {
						continue
					}
					appendReviewField(item, ReviewField{Key: key, Before: format(previous[key]), After: format(next[key])}, &fieldBudget)
				}
			} else if e.Kind == EntryStatement && e.Attributes["format"] == "markdown" {
				previous, next := map[string]string{}, map[string]string{}
				if change.Before != nil {
					data, err := read(*change.Before)
					if err != nil {
						return nil, err
					}
					previous = StatementSections(string(data))
				}
				if change.After != nil {
					data, err := read(*change.After)
					if err != nil {
						return nil, err
					}
					next = StatementSections(string(data))
				}
				keys := map[string]bool{}
				for key := range previous {
					keys[key] = true
				}
				for key := range next {
					keys[key] = true
				}
				ordered := []string{}
				for key := range keys {
					ordered = append(ordered, key)
				}
				sort.Strings(ordered)
				for _, key := range ordered {
					if previous[key] != next[key] {
						appendReviewField(item, ReviewField{Key: key, Before: previous[key], After: next[key]}, &fieldBudget)
					}
				}
			} else {
				old, new := "未添加", "已移除"
				if change.Before != nil {
					old = fmt.Sprintf("%d 字节", change.Before.Blob.Bytes)
				}
				if change.After != nil {
					new = fmt.Sprintf("%d 字节 · %s", change.After.Blob.Bytes, change.Kind)
				}
				appendReviewField(item, ReviewField{Key: labels[e.ID], Before: old, After: new}, &fieldBudget)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Label < result[j].Label
	})
	return result, nil
}
func appendReviewField(item *ReviewItem, field ReviewField, budget *int) {
	clip := func(value string) string {
		if len(value) <= 2048 {
			return value
		}
		item.Truncated = true
		value = value[:2048]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
		return value + "…"
	}
	field.Before = clip(field.Before)
	field.After = clip(field.After)
	size := len(field.Before) + len(field.After) + len(field.Key)
	if size > *budget {
		item.Truncated = true
		return
	}
	*budget -= size
	item.Fields = append(item.Fields, field)
}
func reviewValue(value any, labels map[string]string) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		if label, ok := labels[v]; ok {
			return label
		}
		return v
	case []any:
		parts := []string{}
		for _, x := range v {
			parts = append(parts, reviewValue(x, labels))
		}
		return strings.Join(parts, "、")
	case map[string]any:
		keys := []string{}
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := []string{}
		for _, key := range keys {
			parts = append(parts, key+": "+reviewValue(v[key], labels))
		}
		return strings.Join(parts, "\n")
	default:
		data, _ := json.Marshal(value)
		return string(data)
	}
}

// A statement remains editable Markdown, while headings are reviewable parts.
func StatementSections(text string) map[string]string {
	result := map[string]string{}
	key := "题目描述"
	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if fence == "" {
				fence = trimmed[:3]
			} else if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		}
		if fence == "" && strings.HasPrefix(trimmed, "#") {
			heading := strings.TrimLeft(trimmed, "#")
			if len(heading) < len(trimmed) && strings.HasPrefix(heading, " ") {
				if len(trimmed)-len(heading) == 1 {
					result["题目标题"] = strings.TrimSpace(heading)
					key = "题目描述"
					continue
				}
				key = strings.TrimSpace(heading)
				if _, ok := result[key]; !ok {
					result[key] = ""
				}
				continue
			}
		}
		result[key] += line + "\n"
	}
	for key, value := range result {
		result[key] = strings.TrimSpace(value)
	}
	return result
}
