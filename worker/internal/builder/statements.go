package builder

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/RimuruChan/Vertex/worker/internal/statement"
)

func (check *frozenBuild) renderStatements(ctx context.Context) error {
	samples := []statement.Sample{}
	for index, test := range check.job.Check.Tests {
		if test.Definition.IsSample {
			samples = append(samples, statement.Sample{Input: filepath.Join(check.directory, fmt.Sprintf("%d.in", index+1)), Answer: filepath.Join(check.directory, fmt.Sprintf("%d.out", index+1))})
		}
	}
	for index, document := range check.job.Check.Statements {
		check.report.Stage = "statement"
		check.reporter.Stage("statement", index, len(check.job.Check.Statements))
		files := map[string]string{}
		for _, file := range append([]FrozenFile{document.Source}, document.Files...) {
			if _, exists := files[file.Path]; exists {
				return fmt.Errorf("duplicate statement file: %s", file.Path)
			}
			stored, err := check.content(ctx, file.Blob)
			if err != nil {
				return err
			}
			files[file.Path] = stored
		}
		output := filepath.Join(check.directory, "statement-"+document.ID+".pdf")
		if err := statement.Render(ctx, check.builder.sandbox, check.directory, document.Source.Path, files, output, document.Dialect, statement.Options{Title: document.Title, Samples: samples}); err != nil {
			return fmt.Errorf("题面 %s：%w", document.Language, err)
		}
		ref, err := fileReference(output)
		if err != nil {
			return err
		}
		check.manifest.Statements = append(check.manifest.Statements, ArtifactStatement{ID: document.ID, PDF: ref})
		check.reporter.Stage("statement", index+1, len(check.job.Check.Statements))
	}
	return nil
}
