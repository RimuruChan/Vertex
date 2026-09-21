package application

import (
	"context"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (service *Workbench) PreviewStatement(ctx context.Context, id string, input domain.StatementPreviewInput) (*domain.DraftStatementPreview, error) {
	if len(input.Content) > 1<<20 || !utf8.ValidString(input.Content) || strings.ContainsRune(input.Content, 0) || input.Revision < 0 {
		return nil, domain.InvalidInput("题面预览内容无效")
	}
	var tree domain.ContentTree
	if input.Revision > 0 {
		_, snapshot, err := service.revisions.Revision(ctx, id, input.Revision)
		if err != nil {
			return nil, err
		}
		tree = snapshot
	} else {
		copy, err := service.revisions.WorkingCopy(ctx, id)
		if err != nil {
			return nil, err
		}
		if input.ETag == "" || copy.ETag != input.ETag {
			return nil, domain.ErrWorkingCopyConflict
		}
		tree = copy.Tree
	}
	entries := map[string]domain.TreeEntry{}
	for _, entry := range tree.Entries {
		entries[entry.ID] = entry
	}
	statement, ok := entries[input.EntryID]
	if !ok || statement.Kind != domain.EntryStatement || statement.Attributes["format"] != "markdown" {
		return nil, domain.InvalidInput("请选择 Markdown 题面")
	}
	result := &domain.DraftStatementPreview{Content: input.Content, Warnings: []string{}}
	metadata, err := service.readDocument(ctx, id, entries["problem"])
	if err != nil {
		return nil, err
	}
	samples := []domain.Sample{}
	remaining := int64(256 << 10)
	readSample := func(entry domain.TreeEntry) (string, error) {
		if entry.Blob.Bytes > 64<<10 || entry.Blob.Bytes > remaining {
			return "", domain.InvalidInput("样例过大，请下载查看完整内容")
		}
		remaining -= entry.Blob.Bytes
		_, reader, err := service.revisions.Blob(ctx, id, entry.Blob.SHA256)
		if err != nil {
			return "", err
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, (64<<10)+1))
		if err != nil {
			return "", err
		}
		if err = domain.ValidateBlob(entry.Blob, data); err != nil {
			return "", err
		}
		if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
			return "", domain.InvalidInput("二进制样例请下载查看")
		}
		return string(data), nil
	}
	for _, testID := range metadata.Metadata.TestOrder {
		entry := entries[testID]
		if entry.Kind != domain.EntryTest {
			continue
		}
		view, err := service.readDocument(ctx, id, entry)
		if err != nil {
			return nil, err
		}
		if !view.Test.IsSample {
			continue
		} // Never read secret testcase input/answer bytes.
		result.SampleCount++
		sample := domain.Sample{Index: result.SampleCount}
		inputFile, answerFile := entries[view.Test.Input.Entry], entries[view.Test.Answer.Entry]
		if view.Test.Input.Kind != "file" || view.Test.Answer.Kind != "file" || inputFile.Kind != domain.EntryInput || answerFile.Kind != domain.EntryAnswer {
			result.Warnings = append(result.Warnings, "样例「"+view.Test.Name+"」需先生成输入和答案；完整结果请在检查后查看。")
			sample.Input = "（运行检查后可查看）"
			sample.Answer = sample.Input
		} else {
			sample.Input, err = readSample(inputFile)
			if err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				sample.Input = "（无法内联预览）"
			}
			sample.Answer, err = readSample(answerFile)
			if err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				sample.Answer = "（无法内联预览）"
			}
		}
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		result.Warnings = append(result.Warnings, "尚未添加公开样例，可在测试数据中添加并标记为样例。")
	}
	result.Content, err = domain.PlaceSamples(input.Content, statement.Attributes["language"], samples, true)
	if err != nil {
		return nil, err
	}
	return result, nil
}
