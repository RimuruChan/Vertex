package packages

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

type PutContent func(io.Reader) (domain.BlobRef, error)
type importer struct {
	storedBytes int64
	archive     *archive
	put         PutContent
	plan        domain.ImportPlan
	metadata    domain.PackageMetadata
	refs        map[string]domain.TreeEntry
}

func Import(data []byte, options domain.ImportOptions, put PutContent) (*domain.ImportPlan, error) {
	a, err := readArchive(data)
	if err != nil {
		return nil, err
	}
	if err := a.selectRoot(); err != nil {
		return nil, err
	}
	value := &importer{archive: a, put: put, refs: map[string]domain.TreeEntry{}, plan: domain.ImportPlan{Scope: "problem", ArchiveHash: domain.Digest(data), Issues: []domain.CompatibilityIssue{}, Tree: domain.ContentTree{Entries: []domain.TreeEntry{}}, CanApply: true}}
	switch a.marker() {
	case "vertex-package.json":
		err = value.native()
	case "problem.yaml":
		err = value.kattis(options)
	case "problem.xml":
		err = value.polygon()
	default:
		err = value.flatData()
	}
	if err != nil {
		return nil, err
	}
	if options.Format != "" && options.Format != value.plan.Format {
		return nil, invalid("所选格式与题包内容不一致")
	}
	tree, err := value.plan.Tree.Canonical()
	if err != nil {
		return nil, err
	}
	value.plan.Tree = tree
	value.plan.FileCount = len(tree.Entries)
	return &value.plan, nil
}

func (value *importer) issue(severity, code, file, message string) {
	value.plan.Issues = append(value.plan.Issues, domain.CompatibilityIssue{Severity: severity, Code: code, Path: file, Message: message})
	if severity == "error" {
		value.plan.CanApply = false
	}
}
func (value *importer) require(code, file, message, stage string) {
	for _, requirement := range value.metadata.Requirements {
		if requirement.Code == code && requirement.Message == message {
			return
		}
	}
	value.metadata.Requirements = append(value.metadata.Requirements, domain.MaterialRequirement{Code: code, Path: file, Message: message, Stage: stage})
	value.issue("blocking", code, file, message)
}
func stableID(kind, name string) string { return kind + "-" + domain.Digest([]byte(name)) }
func (value *importer) file(name, kind string, attributes map[string]string) (domain.TreeEntry, error) {
	if existing, ok := value.refs[name]; ok {
		if existing.Kind != kind {
			return domain.TreeEntry{}, invalid("材料用途冲突：%s", name)
		}
		return existing, nil
	}
	stream, err := value.archive.open(name)
	if err != nil {
		return domain.TreeEntry{}, err
	}
	ref, err := value.storeFile(stream)
	stream.Close()
	if err != nil {
		return domain.TreeEntry{}, err
	}
	if attributes == nil {
		attributes = map[string]string{}
	}
	entry := domain.TreeEntry{ID: stableID("file", name), Path: name, Kind: kind, Blob: ref, Attributes: attributes}
	value.refs[name] = entry
	value.plan.Tree.Entries = append(value.plan.Tree.Entries, entry)
	return entry, nil
}

func (value *importer) storeFile(stream io.Reader) (domain.BlobRef, error) {
	ref, err := value.put(io.LimitReader(stream, MaxEntryBytes+1))
	if err != nil {
		return ref, err
	}
	if ref.Bytes > MaxEntryBytes || ref.Bytes > MaxExpandedBytes-value.storedBytes {
		return ref, domain.ErrPackageTooBig
	}
	value.storedBytes += ref.Bytes
	return ref, nil
}
func (value *importer) document(id, name, kind string, document any) error {
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	data, err = domain.NormalizeMaterial(kind, data)
	if err != nil {
		return err
	}
	ref, err := value.put(bytes.NewReader(data))
	if err != nil {
		return err
	}
	value.plan.Tree.Entries = append(value.plan.Tree.Entries, domain.TreeEntry{ID: id, Path: name, Kind: kind, Blob: ref, Attributes: map[string]string{"format": "json"}})
	return nil
}
func (value *importer) preserveRemaining() error {
	for _, name := range value.archive.names {
		if _, ok := value.refs[name]; ok {
			continue
		}
		if strings.HasPrefix(name, "vertex/") {
			return invalid("外部题包占用了 Vertex 材料目录：%s", name)
		}
		kind := domain.EntryResource
		attrs := map[string]string{"originFormat": value.plan.Format}
		if strings.HasPrefix(name, "attachments/") {
			kind = domain.EntryAsset
			attrs["visibility"] = "public"
		}
		if strings.HasPrefix(name, "statement/") || strings.HasPrefix(name, "problem_statement/") {
			switch strings.ToLower(path.Ext(name)) {
			case ".tex", ".sty", ".cls":
				attrs["purpose"] = "statement-support"
			case ".png", ".jpg", ".jpeg", ".pdf":
				kind = domain.EntryAsset
				attrs["visibility"], attrs["purpose"] = "public", "statement-support"
			}
		}
		if _, err := value.file(name, kind, attrs); err != nil {
			return err
		}
	}
	return nil
}

func (value *importer) native() error {
	data, err := value.archive.read("vertex-package.json", 8<<20)
	if err != nil {
		return err
	}
	var manifest struct {
		SchemaVersion int                `json:"schemaVersion"`
		Tree          domain.ContentTree `json:"tree"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !json.Valid(data) || decoder.Decode(&manifest) != nil || manifest.SchemaVersion != 1 {
		return invalid("无效的 Vertex 归档")
	}
	tree, err := manifest.Tree.Canonical()
	if err != nil {
		return err
	}
	seen := map[string]bool{"vertex-package.json": true}
	verified := map[string]domain.BlobRef{}
	for _, entry := range tree.Entries {
		name := path.Join("blobs", entry.Blob.SHA256)
		if seen[name] {
			if verified[name] != entry.Blob {
				return invalid("同一内容摘要声明了不同长度：%s", entry.Path)
			}
			continue
		}
		stream, err := value.archive.open(name)
		if err != nil {
			return err
		}
		ref, err := value.storeFile(stream)
		stream.Close()
		if err != nil {
			return err
		}
		if ref != entry.Blob {
			return invalid("归档材料摘要不匹配：%s", entry.Path)
		}
		seen[name] = true
		verified[name] = ref
	}
	for _, name := range value.archive.names {
		if !seen[name] {
			return invalid("Vertex 归档包含未声明文件：%s", name)
		}
	}
	value.plan.Format = "vertex"
	value.plan.Tree = tree
	return nil
}
