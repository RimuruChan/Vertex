package domain

import (
	"crypto/rand"
	"path"
	"strings"
)

type ProgramSourceEdit struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	RelativeName string  `json:"relativeName"`
	BaseHash     string  `json:"baseHash"`
	Blob         BlobRef `json:"blob"`
}
type ProgramSaveInput struct {
	ETag     string              `json:"etag"`
	ID       string              `json:"id"`
	BaseHash string              `json:"baseHash"`
	Program  ProgramMaterial     `json:"program"`
	Sources  []ProgramSourceEdit `json:"sources"`
}
type ProgramSaveResult struct {
	Copy    WorkingCopy       `json:"copy"`
	Program ProgramMaterial   `json:"program"`
	Sources []TreeEntry       `json:"sources"`
	Remap   map[string]string `json:"remap"`
}

// ArrangeProgram owns source membership, safe placement and shared-source
// isolation. The UI describes a program rather than replacing the whole tree.
func ArrangeProgram(tree ContentTree, input ProgramSaveInput, programs map[string]ProgramMaterial) (ProgramMaterial, []TreeEntry, map[string]string, error) {
	program := input.Program
	program.Files = append([]string{}, input.Program.Files...)
	remap := map[string]string{}
	fail := func(message string) (ProgramMaterial, []TreeEntry, map[string]string, error) {
		return program, nil, nil, InvalidInput(message)
	}
	if !contentIDPattern.MatchString(input.ID) || input.ID == "problem" || len(input.Sources) < 1 || len(input.Sources) > 200 {
		return fail("程序标识或代码数量无效")
	}
	existing := map[string]TreeEntry{}
	for _, e := range tree.Entries {
		existing[e.ID] = e
	}
	current, exists := existing[input.ID]
	if exists && current.Kind != EntryProgram {
		return fail("程序标识已被其他材料占用")
	}
	if (exists && current.Blob.SHA256 != input.BaseHash) || (!exists && input.BaseHash != "") {
		return program, nil, nil, ErrWorkingCopyConflict
	}
	if exists {
		previous, ok := programs[input.ID]
		if !ok {
			return fail("已有程序无法读取")
		}
		program.Directory = previous.Directory
	}
	seen := map[string]bool{}
	byID := map[string]ProgramSourceEdit{}
	isolate := !exists
	for _, source := range input.Sources {
		if !contentIDPattern.MatchString(source.ID) || seen[source.ID] || source.ID == input.ID || !digestPattern.MatchString(source.Blob.SHA256) || source.Blob.Bytes < 0 {
			return fail("代码标识或内容引用无效")
		}
		seen[source.ID] = true
		byID[source.ID] = source
		if err := ValidatePackagePath(source.RelativeName); err != nil {
			return fail("程序内代码名称无效")
		}
		old, found := existing[source.ID]
		if found && old.Kind != EntrySource {
			return fail("代码标识对应的不是源代码")
		}
		if (found && old.Blob.SHA256 != source.BaseHash) || (!found && source.BaseHash != "") {
			return program, nil, nil, ErrWorkingCopyConflict
		}
		target := source.RelativeName
		if program.Directory != "" {
			target = program.Directory + "/" + target
		}
		for _, e := range tree.Entries {
			if e.ID != source.ID && pathCollision(target, e.Path) {
				isolate = true
			}
		}
		if found && (old.Blob != source.Blob || old.Path != target) {
			for id, p := range programs {
				if id == input.ID {
					continue
				}
				for _, ref := range p.Files {
					if ref == source.ID {
						isolate = true
					}
				}
			}
		}
	}
	if len(program.Files) != len(input.Sources) {
		return fail("程序成员与代码列表不一致")
	}
	for _, id := range program.Files {
		if !seen[id] {
			return fail("程序引用了未提供的代码")
		}
	}
	if isolate {
		program.Directory = "program-" + input.ID + "-" + rand.Text()
		for {
			conflict := false
			for _, e := range tree.Entries {
				if pathCollision(program.Directory, e.Path) {
					conflict = true
				}
			}
			if !conflict {
				break
			}
			program.Directory = "program-" + rand.Text()
		}
	}
	result := make([]TreeEntry, 0, len(input.Sources))
	for _, id := range program.Files {
		source := byID[id]
		sourceID := id
		if isolate {
			if _, found := existing[id]; found {
				sourceID = "source-" + rand.Text()
				remap[id] = sourceID
			}
		}
		location := source.RelativeName
		if program.Directory != "" {
			location = program.Directory + "/" + location
		}
		label := source.Name
		if label == "" {
			label = path.Base(source.RelativeName)
		}
		result = append(result, TreeEntry{ID: sourceID, Kind: EntrySource, Path: location, Blob: source.Blob, Attributes: map[string]string{"language": program.Language, "label": label}})
	}
	for i, id := range program.Files {
		if next, ok := remap[id]; ok {
			program.Files[i] = next
		}
	}
	if next, ok := remap[program.EntryPoint]; ok {
		program.EntryPoint = next
	}
	if program.EntryPoint == "" {
		return fail("程序需要主代码")
	}
	if err := program.Validate(); err != nil {
		return program, nil, nil, err
	}
	if _, err := (ContentTree{Entries: result}).Canonical(); err != nil {
		return program, nil, nil, err
	}
	return program, result, remap, nil
}
func pathCollision(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
