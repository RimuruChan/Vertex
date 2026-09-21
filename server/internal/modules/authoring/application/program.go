package application

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func (service *Workbench) SaveProgram(ctx context.Context, id string, input domain.ProgramSaveInput) (*domain.ProgramSaveResult, error) {
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
	programs := map[string]domain.ProgramMaterial{}
	var current domain.TreeEntry
	for _, entry := range copy.Tree.Entries {
		if entry.Kind != domain.EntryProgram {
			continue
		}
		view, err := service.readDocument(ctx, id, entry)
		if err != nil {
			return nil, err
		}
		programs[entry.ID] = *view.Program
		if entry.ID == input.ID {
			current = entry
		}
	}
	program, sources, remap, err := domain.ArrangeProgram(copy.Tree, input, programs)
	if err != nil {
		return nil, err
	}
	if current.ID == "" {
		current = domain.TreeEntry{ID: input.ID, Kind: domain.EntryProgram, Path: "vertex/programs/" + input.ID + ".json"}
	}
	current.Attributes = map[string]string{"format": "json", "label": program.Name}
	current, err = service.storeDocument(ctx, id, current, program)
	if err != nil {
		return nil, err
	}
	updates := map[string]domain.TreeEntry{current.ID: current}
	for _, source := range sources {
		updates[source.ID] = source
	}
	entries := []domain.TreeEntry{}
	for _, entry := range copy.Tree.Entries {
		if next, ok := updates[entry.ID]; ok {
			entries = append(entries, next)
			delete(updates, entry.ID)
		} else {
			entries = append(entries, entry)
		}
	}
	for _, entry := range updates {
		entries = append(entries, entry)
	}
	next, err := service.Save(ctx, id, input.ETag, domain.ContentTree{Entries: entries})
	if err != nil {
		return nil, err
	}
	return &domain.ProgramSaveResult{Copy: *next, Program: program, Sources: sources, Remap: remap}, nil
}
