package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres/internal/dbgen"
)

type preparedPresentation struct {
	markdown string
	files    []dbgen.CreatePublishedFileParams
}

func (repo *RevisionRepository) presentation(ctx context.Context, q *dbgen.Queries, id, actor, language, artifactPath string, tree domain.ContentTree, snapshot *domain.CheckSnapshot, manifest domain.CheckArtifact) (*preparedPresentation, error) {
	var statement *domain.TreeEntry
	for index := range tree.Entries {
		entry := &tree.Entries[index]
		if entry.Kind == domain.EntryStatement && strings.EqualFold(entry.Attributes["language"], language) {
			statement = entry
			break
		}
	}
	if statement == nil {
		return nil, domain.InvalidInput("selected statement language is missing")
	}
	format := statement.Attributes["format"]
	compiledTeX := format == "tex" && statement.Attributes["dialect"] != "polygon"
	if format == "tex" {
		var rendered *domain.ArtifactStatement
		for index := range manifest.Statements {
			if manifest.Statements[index].ID == statement.ID {
				rendered = &manifest.Statements[index]
				break
			}
		}
		if rendered == nil {
			return nil, domain.InvalidInput("TeX 题面缺少匹配的编译结果，请重新检查后发布")
		}
		pdf, err := repo.artifacts.Read(ctx, id, artifactPath, "statements/"+statement.ID+".pdf", 32<<20)
		if err != nil {
			return nil, err
		}
		if err := domain.ValidateBlob(rendered.PDF, pdf); err != nil {
			return nil, err
		}
		ref, err := repo.blobs.Put(ctx, id, bytes.NewReader(pdf))
		if err != nil {
			return nil, err
		}
		if err := registerBlob(ctx, q, id, actor, ref); err != nil {
			return nil, err
		}
		selected := *statement
		selected.Blob = ref
		selected.Path = strings.TrimSuffix(selected.Path, path.Ext(selected.Path)) + ".pdf"
		statement = &selected
		format = "pdf"
	}
	if format != "markdown" && format != "pdf" {
		return nil, domain.InvalidInput("unsupported statement presentation format")
	}
	body, err := repo.presentationBytes(ctx, id, statement.Blob, 32<<20)
	if err != nil {
		return nil, err
	}
	result := &preparedPresentation{files: []dbgen.CreatePublishedFileParams{}}
	mediaType := "text/markdown; charset=utf-8"
	if format == "markdown" {
		if len(body) > 1<<20 || !utf8.Valid(body) {
			return nil, domain.InvalidInput("Markdown statement must be UTF-8 and at most 1 MiB")
		}
		result.markdown = strings.TrimSpace(string(body))
	} else {
		if !bytes.HasPrefix(body, []byte("%PDF-")) {
			return nil, domain.InvalidInput("statement is not a PDF document")
		}
		mediaType = "application/pdf"
	}
	result.files = append(result.files, dbgen.CreatePublishedFileParams{ProblemID: id, FileID: "statement", Path: statement.Path, Filename: "problem-" + language + map[string]string{"markdown": ".md", "pdf": ".pdf"}[format], MediaType: mediaType, Purpose: "statement", BlobSha256: statement.Blob.SHA256, ByteSize: statement.Blob.Bytes, IsBinary: format == "pdf"})
	total := statement.Blob.Bytes
	for _, entry := range tree.Entries {
		if entry.Kind != domain.EntryAsset || entry.Attributes["visibility"] != "" && entry.Attributes["visibility"] != "public" {
			continue
		}
		data, err := repo.presentationBytes(ctx, id, entry.Blob, 64<<20)
		if err != nil {
			return nil, err
		}
		kind := http.DetectContentType(data)
		switch kind {
		case "image/png", "image/jpeg", "image/gif", "image/webp", "application/pdf":
		default:
			kind = "application/octet-stream"
		}
		result.files = append(result.files, dbgen.CreatePublishedFileParams{ProblemID: id, FileID: "asset-" + entry.ID, Path: entry.Path, Filename: path.Base(entry.Path), MediaType: kind, Purpose: "asset", BlobSha256: entry.Blob.SHA256, ByteSize: entry.Blob.Bytes, IsBinary: true})
		total += entry.Blob.Bytes
		if total > 256<<20 || len(result.files) > 1000 {
			return nil, domain.InvalidInput("public presentation files exceed publication limits")
		}
	}
	smallSamples := []domain.Sample{}
	sampleIndex := 0
	allInline := format == "markdown"
	for index, test := range snapshot.Tests {
		if !test.Definition.IsSample {
			continue
		}
		sampleIndex++
		expected := manifest.Tests[index]
		input, err := repo.artifacts.Read(ctx, id, artifactPath, fmt.Sprintf("%d.in", index+1), 64<<20)
		if err != nil {
			return nil, err
		}
		answer, err := repo.artifacts.Read(ctx, id, artifactPath, fmt.Sprintf("%d.out", index+1), 64<<20)
		if err != nil {
			return nil, err
		}
		if err := domain.ValidateBlob(expected.Input, input); err != nil {
			return nil, err
		}
		if err := domain.ValidateBlob(expected.Answer, answer); err != nil {
			return nil, err
		}
		inline := (format == "markdown" || compiledTeX) && len(input) <= 8192 && len(answer) <= 8192 && textualSample(input) && textualSample(answer)
		allInline = allInline && inline
		if inline {
			smallSamples = append(smallSamples, domain.Sample{Index: sampleIndex, Input: string(input), Answer: string(answer)})
		} else {
			smallSamples = append(smallSamples, domain.Sample{Index: sampleIndex})
		}
		for _, part := range []struct {
			kind, suffix string
			data         []byte
		}{{"input", "in", input}, {"answer", "ans", answer}} {
			total += int64(len(part.data))
			if total > 256<<20 || len(result.files) >= 1000 {
				return nil, domain.InvalidInput("public presentation files exceed publication limits")
			}
			ref, err := repo.blobs.Put(ctx, id, bytes.NewReader(part.data))
			if err != nil {
				return nil, err
			}
			if err := registerBlob(ctx, q, id, actor, ref); err != nil {
				return nil, err
			}
			binary := !textualSample(part.data)
			preview := ""
			kind := "application/octet-stream"
			if !binary {
				kind = "text/plain; charset=utf-8"
				{
					head := part.data[:min(len(part.data), 4096)]
					for !utf8.Valid(head) {
						head = head[:len(head)-1]
					}
					preview = string(head)
				}
			}
			name := fmt.Sprintf("sample-%d.%s", sampleIndex, part.suffix)
			result.files = append(result.files, dbgen.CreatePublishedFileParams{ProblemID: id, FileID: fmt.Sprintf("sample-%d-%s", sampleIndex, part.kind), Path: "samples/" + name, Filename: name, MediaType: kind, Purpose: "sample-" + part.kind, BlobSha256: ref.SHA256, ByteSize: ref.Bytes, SampleIndex: sampleIndex, Preview: preview, Truncated: !binary && len(part.data) > len(preview), IsBinary: binary, Embedded: inline})
		}
	}
	for index := range result.files {
		file := &result.files[index]
		if file.SampleIndex > 0 && format == "markdown" {
			file.Embedded = allInline
			if allInline {
				file.Preview = ""
				file.Truncated = false
			}
		}
	}
	if format == "markdown" {
		result.markdown, err = domain.PlaceSamples(result.markdown, language, smallSamples, allInline)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
func textualSample(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, value := range data {
		if value < 32 && value != '\n' && value != '\r' && value != '\t' || value == 127 {
			return false
		}
	}
	return true
}
func (repo *RevisionRepository) presentationBytes(ctx context.Context, id string, ref domain.BlobRef, limit int64) ([]byte, error) {
	if ref.Bytes > limit {
		return nil, domain.ErrPackageTooBig
	}
	stream, err := repo.blobs.Open(ctx, id, ref)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	body, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateBlob(ref, body); err != nil {
		return nil, err
	}
	return body, nil
}
