package application

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (service *Workbench) readDocument(ctx context.Context, id string, entry domain.TreeEntry) (*domain.MaterialView, error) {
	if entry.Blob.Bytes > 1<<20 {
		return nil, domain.InvalidInput("材料文档过大")
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
	if err = domain.ValidateBlob(entry.Blob, data); err != nil {
		return nil, err
	}
	return domain.DecodeMaterial(entry, data)
}
func (service *Workbench) storeDocument(ctx context.Context, id string, entry domain.TreeEntry, value any) (domain.TreeEntry, error) {
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
func (service *Workbench) Generate(ctx context.Context, id string, input domain.GenerationInput) (*domain.GenerationResult, error) {
	if err := service.revisions.AuthorizeEdit(ctx, id); err != nil {
		return nil, err
	}
	copy, err := service.revisions.WorkingCopy(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.ETag == "" || copy.ETag != input.ETag {
		return nil, domain.ErrWorkingCopyConflict
	}
	if copy.MergeID != "" {
		return nil, domain.ErrMergeRequired
	}
	cases, err := domain.ExpandGeneration(input.ID, input.Plan)
	if err != nil {
		return nil, err
	}
	entries := map[string]domain.TreeEntry{}
	for _, e := range copy.Tree.Entries {
		entries[e.ID] = e
	}
	if e, exists := entries[input.ID]; exists && e.Kind != domain.EntryGeneration {
		return nil, domain.InvalidInput("此标识已被其他材料使用")
	}
	for ref, role := range map[string]string{input.Plan.Generator: "generator", input.Plan.Solution: "solution"} {
		entry, exists := entries[ref]
		if !exists || entry.Kind != domain.EntryProgram {
			return nil, domain.InvalidInput("请选择现有的生成器和标准解")
		}
		view, err := service.readDocument(ctx, id, entry)
		if err != nil {
			return nil, err
		}
		if view.Program.Role != role {
			return nil, domain.InvalidInput("程序用途不匹配")
		}
		if role == "solution" && (len(view.Program.ExpectedVerdicts) != 1 || view.Program.ExpectedVerdicts[0] != "Accepted") {
			return nil, domain.InvalidInput("生成答案需要预期通过的正确参考解")
		}
	}
	if input.Plan.Generator == input.Plan.Solution {
		return nil, domain.InvalidInput("生成器和标准解必须是不同的程序")
	}
	if input.Plan.Group != "" && entries[input.Plan.Group].Kind != domain.EntryGroup {
		return nil, domain.InvalidInput("测试组不存在")
	}
	result := &domain.GenerationResult{ID: input.ID, Cases: cases}
	if input.Preview {
		return result, nil
	}
	metadata, err := service.readDocument(ctx, id, entries["problem"])
	if err != nil {
		return nil, err
	}
	if metadata.Metadata == nil {
		return nil, domain.InvalidInput("缺少基本设置")
	}
	removed := map[string]bool{}
	next := []domain.TreeEntry{}
	for _, entry := range copy.Tree.Entries {
		if entry.Kind == domain.EntryTest && entry.Attributes["generationPlan"] == input.ID {
			removed[entry.ID] = true
			continue
		}
		if entry.ID != input.ID && entry.ID != "problem" {
			next = append(next, entry)
		}
	}
	generatedOrder := []string{}
	for _, item := range cases {
		generatedOrder = append(generatedOrder, item.ID)
	}
	order := []string{}
	inserted := false
	for _, testID := range metadata.Metadata.TestOrder {
		if removed[testID] {
			if !inserted {
				order = append(order, generatedOrder...)
				inserted = true
			}
			continue
		}
		order = append(order, testID)
	}
	if !inserted {
		order = append(order, generatedOrder...)
	}
	for _, item := range cases {
		if _, exists := entries[item.ID]; exists && !removed[item.ID] {
			return nil, domain.InvalidInput("生成测试标识冲突")
		}
		value := domain.TestMaterial{SchemaVersion: domain.MaterialSchemaVersion, Name: item.Name, Group: input.Plan.Group, Input: domain.TestInput{Kind: "generator", Generator: input.Plan.Generator, Arguments: item.Arguments}, Answer: domain.TestAnswer{Kind: "solution", Solution: input.Plan.Solution}, Description: "由生成方案「" + input.Plan.Name + "」展开"}
		entry := domain.TreeEntry{ID: item.ID, Kind: domain.EntryTest, Path: "vertex/tests/" + item.ID + ".json", Attributes: map[string]string{"format": "json", "label": item.Name, "generationPlan": input.ID, "generationRule": item.RuleID}}
		entry, err = service.storeDocument(ctx, id, entry, value)
		if err != nil {
			return nil, err
		}
		next = append(next, entry)
	}
	metadata.Metadata.TestOrder = order
	meta, err := service.storeDocument(ctx, id, entries["problem"], metadata.Metadata)
	if err != nil {
		return nil, err
	}
	next = append(next, meta)
	plan := domain.TreeEntry{ID: input.ID, Kind: domain.EntryGeneration, Path: "vertex/generation/" + input.ID + ".json", Attributes: map[string]string{"format": "json", "label": input.Plan.Name}}
	plan, err = service.storeDocument(ctx, id, plan, input.Plan)
	if err != nil {
		return nil, err
	}
	next = append(next, plan)
	result.Copy, err = service.Save(ctx, id, input.ETag, domain.ContentTree{Entries: next})
	return result, err
}
