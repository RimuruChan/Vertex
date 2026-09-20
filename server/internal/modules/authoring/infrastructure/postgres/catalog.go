package postgres

import (
	"context"
	"io"
	"sort"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

func (repo *RevisionRepository) Materials(ctx context.Context, id string, input domain.MaterialQuery) (*domain.MaterialPage, error) {
	if input.Revision < 0 || input.Limit < 1 || input.Limit > 100 || (input.Kind != domain.EntryTest && input.Kind != domain.EntryProgram && input.Kind != domain.EntryGroup && input.Kind != domain.EntryValidation) {
		return nil, domain.InvalidInput("invalid material page selection")
	}
	var result *domain.MaterialPage
	err := repo.transaction(ctx, id, false, func(q *dbgen.Queries, actor string) error {
		var tree domain.ContentTree
		var etag string
		var err error
		if input.Revision > 0 {
			tree, etag, err = selectCheckTree(ctx, q, id, actor, input.Revision, input.ETag)
		} else {
			var copy *domain.WorkingCopy
			copy, _, err = readCopy(ctx, q, id, actor)
			if err == nil {
				tree, etag = copy.Tree, copy.ETag
				if input.ETag != "" && input.ETag != etag {
					err = domain.ErrWorkingCopyConflict
				}
			}
		}
		if err != nil {
			return err
		}
		read := func(entry domain.TreeEntry) (*domain.MaterialView, error) {
			if entry.Blob.Bytes > 1<<20 {
				return nil, domain.InvalidInput("材料文档超过读取限制")
			}
			reader, err := repo.blobs.Open(ctx, id, entry.Blob)
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
			reader.Close()
			if err != nil {
				return nil, err
			}
			if err := domain.ValidateBlob(entry.Blob, data); err != nil {
				return nil, err
			}
			return domain.DecodeMaterial(entry, data)
		}
		entries := []domain.TreeEntry{}
		order := map[string]int{}
		for _, entry := range tree.Entries {
			if entry.Kind == input.Kind {
				entries = append(entries, entry)
			}
			if input.Kind == domain.EntryTest && entry.ID == "problem" {
				view, err := read(entry)
				if err == nil && view.Metadata != nil {
					for index, id := range view.Metadata.TestOrder {
						order[id] = index + 1
					}
				}
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			if input.Kind == domain.EntryTest {
				a, b := order[entries[i].ID], order[entries[j].ID]
				if a != b {
					if a == 0 {
						return false
					}
					if b == 0 {
						return true
					}
					return a < b
				}
			}
			if entries[i].Path != entries[j].Path {
				return entries[i].Path < entries[j].Path
			}
			return entries[i].ID < entries[j].ID
		})
		start := 0
		if input.After != "" {
			start = -1
			for index, entry := range entries {
				if entry.ID == input.After {
					start = index + 1
					break
				}
			}
			if start < 0 {
				return domain.InvalidInput("material cursor no longer exists")
			}
		}
		result = &domain.MaterialPage{ETag: etag, Revision: input.Revision, Items: []domain.MaterialView{}, Total: len(entries)}
		var bytes int64
		for _, entry := range entries[start:] {
			if len(result.Items) >= input.Limit || (len(result.Items) > 0 && bytes+entry.Blob.Bytes > 4<<20) {
				result.Next = result.Items[len(result.Items)-1].Entry.ID
				break
			}
			view, err := read(entry)
			if err != nil {
				view = &domain.MaterialView{Entry: entry, Error: "材料文档无效或无法读取，请打开材料检查"}
			}
			view.Position = start + len(result.Items) + 1
			result.Items = append(result.Items, *view)
			bytes += entry.Blob.Bytes
		}
		return nil
	})
	return result, err
}
