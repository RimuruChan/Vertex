package application

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (service *Workbench) Materials(ctx context.Context, id string, input domain.MaterialQuery) (*domain.MaterialPage, error) {
	return service.revisions.Materials(ctx, id, input)
}

// Stage all descriptor changes, then perform one CAS of the complete copy.
// Uploaded blobs may remain on failure; no partial material update is visible.
func (service *Workbench) Batch(ctx context.Context, id string, input domain.MaterialBatch) (*domain.WorkingCopy, error) {
	if input.ETag == "" || len(input.DeleteIDs)+len(input.TestIDs) > 500 || (input.Patch != nil && len(input.TestIDs) == 0) || (len(input.TestIDs) > 0 && input.Patch == nil) {
		return nil, domain.InvalidInput("invalid batch selection")
	}
	copy, err := service.revisions.WorkingCopy(ctx, id)
	if err != nil {
		return nil, err
	}
	if copy.ETag != input.ETag {
		return nil, domain.ErrWorkingCopyConflict
	}
	if copy.MergeID != "" {
		return nil, domain.ErrMergeRequired
	}
	byID := map[string]domain.TreeEntry{}
	for _, entry := range copy.Tree.Entries {
		byID[entry.ID] = entry
	}
	deleted := map[string]bool{}
	for _, entryID := range input.DeleteIDs {
		if entryID == "problem" {
			return nil, domain.InvalidInput("不能删除基本设置")
		}
		if _, exists := byID[entryID]; !exists || deleted[entryID] {
			return nil, domain.InvalidInput("删除选择包含不存在或重复的材料")
		}
		deleted[entryID] = true
	}
	read := func(entry domain.TreeEntry) (*domain.MaterialView, error) {
		if entry.Blob.Bytes > 1<<20 {
			return nil, domain.InvalidInput("材料文档超过编辑限制")
		}
		_, reader, err := service.revisions.Blob(ctx, id, entry.Blob.SHA256)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if err != nil {
			return nil, err
		}
		return domain.DecodeMaterial(entry, data)
	}
	store := func(entry domain.TreeEntry, value any) (domain.TreeEntry, error) {
		data, err := json.Marshal(value)
		if err != nil {
			return entry, err
		}
		data, err = domain.NormalizeMaterial(entry.Kind, data)
		if err != nil {
			return entry, err
		}
		entry.Blob, err = service.revisions.Upload(ctx, id, strings.NewReader(string(data)))
		return entry, err
	}
	updated := map[string]domain.TreeEntry{}
	for _, entryID := range input.TestIDs {
		entry, exists := byID[entryID]
		if !exists || entry.Kind != domain.EntryTest || deleted[entryID] {
			return nil, domain.InvalidInput("测试点选择无效")
		}
		if _, exists := updated[entryID]; exists {
			return nil, domain.InvalidInput("测试点选择重复")
		}
		view, err := read(entry)
		if err != nil {
			return nil, err
		}
		if input.Patch.IsSample != nil {
			view.Test.IsSample = *input.Patch.IsSample
		}
		if input.Patch.IsPretest != nil {
			view.Test.IsPretest = *input.Patch.IsPretest
		}
		if input.Patch.Group != nil {
			group := *input.Patch.Group
			if group != "" && (byID[group].Kind != domain.EntryGroup || deleted[group]) {
				return nil, domain.InvalidInput("分组不存在或将被删除")
			}
			view.Test.Group = group
		}
		if input.Patch.Points != nil {
			view.Test.Points = *input.Patch.Points
		}
		if input.Patch.TimeLimitMs != nil {
			view.Test.TimeLimitMs = *input.Patch.TimeLimitMs
		}
		if input.Patch.MemoryLimitKB != nil {
			view.Test.MemoryLimitKB = *input.Patch.MemoryLimitKB
		}
		entry, err = store(entry, view.Test)
		if err != nil {
			return nil, err
		}
		updated[entryID] = entry
	}
	metadata, exists := byID["problem"]
	if !exists {
		return nil, domain.InvalidInput("缺少基本设置")
	}
	view, err := read(metadata)
	if err != nil {
		return nil, err
	}
	order := []string{}
	for _, entryID := range view.Metadata.TestOrder {
		if !deleted[entryID] {
			order = append(order, entryID)
		}
	}
	if input.TestOrder != nil {
		seen := map[string]bool{}
		for _, entryID := range *input.TestOrder {
			if seen[entryID] || byID[entryID].Kind != domain.EntryTest || deleted[entryID] {
				return nil, domain.InvalidInput("测试顺序无效")
			}
			seen[entryID] = true
		}
		for _, entry := range copy.Tree.Entries {
			if entry.Kind == domain.EntryTest && !deleted[entry.ID] && !seen[entry.ID] {
				return nil, domain.InvalidInput("测试顺序必须包含全部保留的测试点")
			}
		}
		order = *input.TestOrder
	}
	view.Metadata.TestOrder = order
	metadata, err = store(metadata, view.Metadata)
	if err != nil {
		return nil, err
	}
	updated["problem"] = metadata
	entries := []domain.TreeEntry{}
	for _, entry := range copy.Tree.Entries {
		if deleted[entry.ID] {
			continue
		}
		if next, ok := updated[entry.ID]; ok {
			entry = next
		}
		entries = append(entries, entry)
	}
	return service.Save(ctx, id, input.ETag, domain.ContentTree{Entries: entries})
}
