package application

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (service *Workbench) Inspect(ctx context.Context, id string, revision int64, etag string) (*domain.MaterialInspection, error) {
	return service.revisions.Inspect(ctx, id, revision, etag)
}

func (service *Workbench) updateTestOrder(ctx context.Context, id string, copy *domain.WorkingCopy, testID string, remove bool) error {
	for index, entry := range copy.Tree.Entries {
		if entry.ID != "problem" || entry.Kind != domain.EntryMetadata {
			continue
		}
		if entry.Blob.Bytes > 1<<20 {
			return domain.InvalidInput("problem metadata exceeds the editor limit")
		}
		_, reader, err := service.revisions.Blob(ctx, id, entry.Blob.SHA256)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		reader.Close()
		if err != nil {
			return err
		}
		view, err := domain.DecodeMaterial(entry, data)
		if err != nil {
			return err
		}
		order := []string{}
		found := false
		for _, value := range view.Metadata.TestOrder {
			if value == testID {
				found = true
				if remove {
					continue
				}
			}
			order = append(order, value)
		}
		if !found && !remove {
			order = append(order, testID)
		}
		view.Metadata.TestOrder = order
		data, err = json.Marshal(view.Metadata)
		if err != nil {
			return err
		}
		ref, err := service.revisions.Upload(ctx, id, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		copy.Tree.Entries[index].Blob = ref
		return nil
	}
	return domain.InvalidInput("test ordering requires valid problem metadata")
}

func (service *Workbench) Material(ctx context.Context, id, entryID string, revision int64) (*domain.MaterialView, error) {
	if revision < 0 {
		return nil, domain.InvalidInput("revision cannot be negative")
	}
	var tree domain.ContentTree
	if revision == 0 {
		copy, err := service.revisions.WorkingCopy(ctx, id)
		if err != nil {
			return nil, err
		}
		tree = copy.Tree
	} else {
		_, snapshot, err := service.revisions.Revision(ctx, id, revision)
		if err != nil {
			return nil, err
		}
		tree = snapshot
	}
	for _, entry := range tree.Entries {
		if entry.ID != entryID {
			continue
		}
		if entry.Blob.Bytes > 1<<20 {
			return nil, domain.InvalidInput("material document exceeds the editor limit")
		}
		ref, reader, err := service.revisions.Blob(ctx, id, entry.Blob.SHA256)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if err != nil {
			return nil, err
		}
		if err := domain.ValidateBlob(ref, data); err != nil {
			return nil, err
		}
		return domain.DecodeMaterial(entry, data)
	}
	return nil, domain.ErrNotFound
}

func (service *Workbench) Changes(ctx context.Context, id string, from, target int64) (*domain.ContentComparison, error) {
	return service.revisions.Changes(ctx, id, from, target)
}

// SaveEntry saves exactly one authored item. Related material remains untouched,
// and the read token fences the final update after a potentially slow upload.
func (service *Workbench) SaveEntry(ctx context.Context, id, etag string, entry domain.TreeEntry, text *string) (*domain.WorkingCopy, error) {
	copy, err := service.revisions.WorkingCopy(ctx, id)
	if err != nil {
		return nil, err
	}
	if copy.ETag != etag {
		return nil, domain.ErrWorkingCopyConflict
	}
	if copy.MergeID != "" {
		return nil, domain.ErrMergeRequired
	}
	if entry.Kind == domain.EntryMetadata && (entry.ID != "problem" || entry.Path != "vertex/problem.json") {
		return nil, domain.InvalidInput("problem metadata uses the reserved problem entry")
	}
	if text != nil {
		data := []byte(*text)
		if len(data) > 1<<20 || !utf8.Valid(data) {
			return nil, domain.InvalidInput("editor contents must be UTF-8 of at most 1 MiB")
		}
		switch entry.Kind {
		case domain.EntryMetadata, domain.EntryProgram, domain.EntryTest, domain.EntryGroup, domain.EntryValidation:
			data, err = domain.NormalizeMaterial(entry.Kind, data)
			if err != nil {
				return nil, err
			}
		case domain.EntrySource, domain.EntryStatement:
			if entry.Attributes["format"] == "pdf" {
				return nil, domain.InvalidInput("upload PDF statements as files")
			}
			if strings.ContainsRune(*text, 0) {
				return nil, domain.InvalidInput("text source contains a NUL byte")
			}
			data = []byte(strings.ReplaceAll(strings.ReplaceAll(*text, "\r\n", "\n"), "\r", "\n"))
		case domain.EntryInput, domain.EntryAnswer:
			// Test bytes, including line endings, are deliberately unchanged.
		default:
			return nil, domain.InvalidInput("this entry requires a file upload")
		}
		entry.Blob = domain.Reference(data)
		if _, err := (domain.ContentTree{Entries: []domain.TreeEntry{entry}}).Canonical(); err != nil {
			return nil, err
		}
		ref, err := service.revisions.Upload(ctx, id, strings.NewReader(string(data)))
		if err != nil {
			return nil, err
		}
		entry.Blob = ref
	}
	updated := false
	for i := range copy.Tree.Entries {
		if copy.Tree.Entries[i].ID == entry.ID {
			copy.Tree.Entries[i] = entry
			updated = true
			break
		}
	}
	if !updated {
		copy.Tree.Entries = append(copy.Tree.Entries, entry)
	}
	if entry.Kind == domain.EntryTest {
		if err := service.updateTestOrder(ctx, id, copy, entry.ID, false); err != nil {
			return nil, err
		}
	}
	return service.Save(ctx, id, etag, copy.Tree)
}

func (service *Workbench) DeleteEntry(ctx context.Context, id, etag, entryID string) (*domain.WorkingCopy, error) {
	if entryID == "problem" {
		return nil, domain.InvalidInput("problem metadata cannot be deleted")
	}
	copy, err := service.revisions.WorkingCopy(ctx, id)
	if err != nil {
		return nil, err
	}
	if copy.ETag != etag {
		return nil, domain.ErrWorkingCopyConflict
	}
	for i, entry := range copy.Tree.Entries {
		if entry.ID == entryID {
			copy.Tree.Entries = append(copy.Tree.Entries[:i], copy.Tree.Entries[i+1:]...)
			if entry.Kind == domain.EntryTest {
				if err := service.updateTestOrder(ctx, id, copy, entry.ID, true); err != nil {
					return nil, err
				}
			}
			return service.Save(ctx, id, etag, copy.Tree)
		}
	}
	return nil, domain.ErrNotFound
}
