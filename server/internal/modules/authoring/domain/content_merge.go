package domain

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// BlobMerger may merge structured documents or bounded text. It must return
// immutable new blobs to persist before the merged tree becomes reachable.
// Input/answer/binary entries never go through this callback.
type BlobMerger func(kind string, base, local, remote BlobRef) (BlobRef, bool, error)

type ContentConflict struct {
	EntryID string     `json:"entryId"`
	Field   string     `json:"field"`
	Kind    string     `json:"kind"`
	Base    *TreeEntry `json:"base,omitempty"`
	Local   *TreeEntry `json:"local,omitempty"`
	Remote  *TreeEntry `json:"remote,omitempty"`
}

type ContentMerge struct {
	Tree      ContentTree       `json:"tree"`
	Conflicts []ContentConflict `json:"conflicts"`
}

type ContentChange struct {
	EntryID string     `json:"entryId"`
	Kind    string     `json:"kind"`
	Before  *TreeEntry `json:"before,omitempty"`
	After   *TreeEntry `json:"after,omitempty"`
}

// MergeTrees leaves the caller's inputs untouched. The provisional tree keeps
// local values for conflicts, but MUST NOT be committed until every conflict is
// explicitly resolved and the resulting whole tree passes Canonical again.
func MergeTrees(base, local, remote ContentTree, mergeBlob BlobMerger) (ContentMerge, error) {
	b, err := base.Canonical()
	if err != nil {
		return ContentMerge{}, err
	}
	l, err := local.Canonical()
	if err != nil {
		return ContentMerge{}, err
	}
	r, err := remote.Canonical()
	if err != nil {
		return ContentMerge{}, err
	}
	baseMap, localMap, remoteMap := entryMap(b), entryMap(l), entryMap(r)
	result := ContentMerge{Tree: ContentTree{Entries: []TreeEntry{}}, Conflicts: []ContentConflict{}}
	for _, id := range entryIDs(b, l, r) {
		before, mine, theirs := baseMap[id], localMap[id], remoteMap[id]
		switch {
		case entriesEqual(mine, theirs):
			result.keep(mine)
		case entriesEqual(before, mine):
			result.keep(theirs)
		case entriesEqual(before, theirs):
			result.keep(mine)
		case before == nil:
			result.conflict(id, "entry", "add-add", before, mine, theirs)
			result.keep(mine)
		case mine == nil || theirs == nil:
			result.conflict(id, "entry", "delete-modify", before, mine, theirs)
			result.keep(mine)
		default:
			merged := cloneEntry(*mine)
			mergeField := func(field, bv, lv, rv string) string {
				value, ok := mergeValue(bv, lv, rv)
				if !ok {
					result.conflict(id, field, "both-modified", before, mine, theirs)
				}
				return value
			}
			merged.Path = mergeField("path", before.Path, mine.Path, theirs.Path)
			merged.Kind = mergeField("kind", before.Kind, mine.Kind, theirs.Kind)
			merged.Attributes = map[string]string{}
			keys := map[string]bool{}
			for _, attributes := range []map[string]string{before.Attributes, mine.Attributes, theirs.Attributes} {
				for key := range attributes {
					keys[key] = true
				}
			}
			for _, key := range slices.Sorted(maps.Keys(keys)) {
				bv, bp := before.Attributes[key]
				lv, lp := mine.Attributes[key]
				rv, rp := theirs.Attributes[key]
				value, ok := mergeValue(optionalString{bv, bp}, optionalString{lv, lp}, optionalString{rv, rp})
				if !ok {
					result.conflict(id, "attributes."+key, "both-modified", before, mine, theirs)
				}
				if value.Present {
					merged.Attributes[key] = value.Value
				}
			}
			blob, ok := mergeValue(before.Blob, mine.Blob, theirs.Blob)
			if !ok && mergeBlob != nil && before.Kind == mine.Kind && mine.Kind == theirs.Kind && mergeableEntry(*before) && mergeableEntry(*mine) && mergeableEntry(*theirs) {
				blob, ok, err = mergeBlob(mine.Kind, before.Blob, mine.Blob, theirs.Blob)
				if err != nil {
					return ContentMerge{}, err
				}
			}
			if !ok {
				blob = mine.Blob
				result.conflict(id, "blob", "both-modified", before, mine, theirs)
			}
			merged.Blob = blob
			result.keep(&merged)
		}
	}
	// Independent additions or renames may create path collisions even though
	// each individual tree and entry merged successfully.
	paths := map[string]string{}
	for _, entry := range result.Tree.Entries {
		key := strings.ToLower(entry.Path)
		if other, exists := paths[key]; exists {
			result.conflict(entry.ID, "path", "path-collision", baseMap[entry.ID], localMap[entry.ID], remoteMap[entry.ID])
			result.conflict(other, "path", "path-collision", baseMap[other], localMap[other], remoteMap[other])
		}
		paths[key] = entry.ID
	}
	for _, entry := range result.Tree.Entries {
		parts := strings.Split(strings.ToLower(entry.Path), "/")
		for i := 1; i < len(parts); i++ {
			if other, exists := paths[strings.Join(parts[:i], "/")]; exists {
				result.conflict(entry.ID, "path", "path-collision", baseMap[entry.ID], localMap[entry.ID], remoteMap[entry.ID])
				result.conflict(other, "path", "path-collision", baseMap[other], localMap[other], remoteMap[other])
			}
		}
	}
	if len(result.Conflicts) == 0 {
		result.Tree, err = result.Tree.Canonical()
	}
	return result, err
}

func DiffTrees(before, after ContentTree) ([]ContentChange, error) {
	b, err := before.Canonical()
	if err != nil {
		return nil, err
	}
	a, err := after.Canonical()
	if err != nil {
		return nil, err
	}
	bm, am := entryMap(b), entryMap(a)
	changes := []ContentChange{}
	for _, id := range entryIDs(b, a) {
		old, next := bm[id], am[id]
		if entriesEqual(old, next) {
			continue
		}
		kind := "modified"
		if old == nil {
			kind = "added"
		} else if next == nil {
			kind = "deleted"
		} else if old.Path != next.Path {
			kind = "renamed"
		}
		changes = append(changes, ContentChange{EntryID: id, Kind: kind, Before: copyEntryPtr(old), After: copyEntryPtr(next)})
	}
	return changes, nil
}

type optionalString struct {
	Value   string
	Present bool
}

func mergeValue[T comparable](base, local, remote T) (T, bool) {
	if local == remote || remote == base {
		return local, true
	}
	if local == base {
		return remote, true
	}
	return local, false
}

func mergeableEntry(entry TreeEntry) bool {
	switch entry.Kind {
	case EntryStatement:
		switch entry.Attributes["format"] {
		case "markdown", "text", "tex", "json":
			return true
		default:
			return false
		}
	case EntryMetadata, EntryProgram, EntrySource, EntryTest, EntryGroup, EntryValidation, EntryGeneration:
		return true
	default:
		return false
	}
}

func entriesEqual(a, b *TreeEntry) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Path == b.Path && a.Kind == b.Kind && a.Blob == b.Blob && maps.Equal(a.Attributes, b.Attributes)
}

func entryMap(tree ContentTree) map[string]*TreeEntry {
	result := make(map[string]*TreeEntry, len(tree.Entries))
	for i := range tree.Entries {
		result[tree.Entries[i].ID] = &tree.Entries[i]
	}
	return result
}

func entryIDs(trees ...ContentTree) []string {
	ids := map[string]bool{}
	for _, tree := range trees {
		for _, entry := range tree.Entries {
			ids[entry.ID] = true
		}
	}
	return slices.Sorted(maps.Keys(ids))
}

func copyEntryPtr(entry *TreeEntry) *TreeEntry {
	if entry == nil {
		return nil
	}
	copy := cloneEntry(*entry)
	return &copy
}

func (result *ContentMerge) keep(entry *TreeEntry) {
	if entry != nil {
		result.Tree.Entries = append(result.Tree.Entries, cloneEntry(*entry))
	}
}

func (result *ContentMerge) conflict(id, field, kind string, base, local, remote *TreeEntry) {
	for _, existing := range result.Conflicts {
		if existing.EntryID == id && existing.Field == field && existing.Kind == kind {
			return
		}
	}
	result.Conflicts = append(result.Conflicts, ContentConflict{id, field, kind, copyEntryPtr(base), copyEntryPtr(local), copyEntryPtr(remote)})
}

// MergeJSON merges document fields independently, including nested objects.
// Arrays are atomic: silently merging test order or dependency lists can change
// judging semantics. Missing fields are distinct from explicit null values.
func MergeJSON(base, local, remote []byte) ([]byte, bool) {
	var b, l, r any
	for _, item := range []struct {
		data   []byte
		target *any
	}{{base, &b}, {local, &l}, {remote, &r}} {
		decoder := json.NewDecoder(strings.NewReader(string(item.data)))
		decoder.UseNumber()
		if !json.Valid(item.data) || decoder.Decode(item.target) != nil {
			return nil, false
		}
	}
	merged, ok := mergeJSONValue(b, l, r)
	if !ok {
		return nil, false
	}
	encoded, err := json.Marshal(merged)
	return encoded, err == nil
}

func mergeJSONValue(base, local, remote any) (any, bool) {
	equal := func(a, b any) bool {
		aa, _ := json.Marshal(a)
		bb, _ := json.Marshal(b)
		return string(aa) == string(bb)
	}
	if equal(local, remote) || equal(base, remote) {
		return local, true
	}
	if equal(base, local) {
		return remote, true
	}
	b, bok := base.(map[string]any)
	l, lok := local.(map[string]any)
	r, rok := remote.(map[string]any)
	if !bok || !lok || !rok {
		return nil, false
	}
	keys := map[string]bool{}
	for _, values := range []map[string]any{b, l, r} {
		for key := range values {
			keys[key] = true
		}
	}
	result := map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		bv, bp := b[key]
		lv, lp := l[key]
		rv, rp := r[key]
		switch {
		case lp == rp && equal(lv, rv):
			if lp {
				result[key] = lv
			}
		case bp == lp && equal(bv, lv):
			if rp {
				result[key] = rv
			}
		case bp == rp && equal(bv, rv):
			if lp {
				result[key] = lv
			}
		case bp && lp && rp:
			value, ok := mergeJSONValue(bv, lv, rv)
			if !ok {
				return nil, false
			}
			result[key] = value
		default:
			return nil, false
		}
	}
	return result, true
}
