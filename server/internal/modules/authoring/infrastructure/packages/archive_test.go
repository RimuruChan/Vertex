package packages

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func packageZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, body := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func testStore(t *testing.T) (PutContent, map[string][]byte) {
	t.Helper()
	blobs := map[string][]byte{}
	return func(reader io.Reader) (domain.BlobRef, error) {
		data, err := io.ReadAll(io.LimitReader(reader, MaxEntryBytes+1))
		if err != nil {
			return domain.BlobRef{}, err
		}
		ref := domain.Reference(data)
		blobs[ref.SHA256] = data
		return ref, nil
	}, blobs
}

func TestArchiveRejectsTraversalAndCollisions(t *testing.T) {
	for _, files := range []map[string]string{{"../outside": "x"}, {"A.in": "1", "a.in": "2"}, {"dir": "x", "dir/file": "y"}, {"C:/outside": "x"}} {
		if _, err := readArchive(packageZIP(t, files)); err == nil {
			t.Fatalf("unsafe archive accepted: %v", files)
		}
	}
}
func TestArchiveResolvesOnlyInternalFileLinks(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	entry, _ := writer.Create("data/input.in")
	entry.Write([]byte("input\n"))
	header := &zip.FileHeader{Name: "data/alias.in"}
	header.SetMode(os.ModeSymlink | 0777)
	entry, _ = writer.CreateHeader(header)
	entry.Write([]byte("input.in"))
	writer.Close()
	a, err := readArchive(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	body, err := a.read("data/alias.in", 100)
	if err != nil || string(body) != "input\n" {
		t.Fatalf("internal link: %q %v", body, err)
	}
	data.Reset()
	writer = zip.NewWriter(&data)
	header = &zip.FileHeader{Name: "escape"}
	header.SetMode(os.ModeSymlink | 0777)
	entry, _ = writer.CreateHeader(header)
	entry.Write([]byte("../outside"))
	writer.Close()
	a, err = readArchive(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.read("escape", 100); err == nil {
		t.Fatal("external link resolved")
	}
}

func kattisFixture(version string) map[string]string {
	statement := "problem_statement/problem.en.tex"
	body := "\\problemname{Sum}\nRead two integers.\n"
	metadata := "problem_format_version: " + version + "\nname: Sum\nlicense: unknown\nlimits:\n  memory: 256\n"
	if version == "2025-09" {
		statement = "statement/problem.en.md"
		body = "# Sum\nRead two integers.\n"
		metadata += "  time_limit: 1\n"
	}
	return map[string]string{"problem.yaml": metadata, statement: body, "data/sample/01.in": "1 2\n", "data/sample/01.ans": "03\n", "data/secret/01.in": "2 5\n", "data/secret/01.ans": "7\n", "input_validators/validator.cpp": "#include <cstdlib>\nint main(){return 42;}\n", "submissions/accepted/reference/main.cpp": "#include \"include/add.h\"\nint main(){return 0;}\n", "submissions/accepted/reference/include/add.h": "long long add(long long,long long);\n"}
}

func TestKattisImportsPreserveSemanticsAndBytes(t *testing.T) {
	for _, version := range []string{"legacy-icpc", "legacy", "2025-09"} {
		t.Run(version, func(t *testing.T) {
			files := kattisFixture(version)
			wrapped := map[string]string{}
			for name, body := range files {
				wrapped["sum/"+name] = body
			}
			store, blobs := testStore(t)
			plan, err := Import(packageZIP(t, wrapped), domain.ImportOptions{TimeLimitMs: 1000}, store)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.CanApply || plan.Format != "kattis-"+version {
				t.Fatalf("bad plan: %+v", plan)
			}
			var meta domain.PackageMetadata
			var answerFound bool
			for _, entry := range plan.Tree.Entries {
				if entry.ID == "problem" {
					if err := json.Unmarshal(blobs[entry.Blob.SHA256], &meta); err != nil {
						t.Fatal(err)
					}
				}
				if entry.Path == "data/sample/01.ans" {
					answerFound = string(blobs[entry.Blob.SHA256]) == "03\n"
				}
			}
			if !answerFound || meta.ResourceMode != "exact" || meta.Comparison.CaseSensitive || meta.TimeLimitMs != 1000 || meta.MemoryLimitKB != 262144 || len(meta.InputValidators) != 1 || len(meta.TestOrder) != 2 || len(meta.Requirements) != 0 {
				t.Fatalf("import semantics: %+v answer=%v", meta, answerFound)
			}
			inspection, _, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
			if err != nil || !inspection.CanBuild {
				t.Fatalf("imported package is not buildable: %+v %v", inspection, err)
			}
		})
	}
}

func TestUnsupportedKattisSettingsRemainBlocking(t *testing.T) {
	files := kattisFixture("2025-09")
	files["problem.yaml"] += "type: [scoring, multi-pass]\n"
	files["data/secret/test_group.yaml"] = "score_aggregation: min\nmax_score: 30\n"
	store, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CanApply {
		t.Fatal("unsupported materials should be preservable as a draft")
	}
	inspection, _, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil {
		t.Fatal(err)
	}
	if inspection.CanBuild {
		t.Fatal("unsupported scoring/multi-pass semantics were silently dropped")
	}
	preserved := false
	for _, entry := range plan.Tree.Entries {
		if entry.Path == "data/secret/test_group.yaml" {
			preserved = string(blobs[entry.Blob.SHA256]) == files[entry.Path]
		}
	}
	if !preserved {
		t.Fatal("unsupported original config was lost")
	}
}

func TestYAMLRejectsAmbiguousOrRecursiveConfiguration(t *testing.T) {
	for _, text := range []string{"name: a\nname: b\n", "x: &x [*x]\n", "name: a\n---\nname: b\n"} {
		if _, err := readYAML([]byte(text)); err == nil {
			t.Fatalf("bad YAML accepted: %s", text)
		}
	}
	if _, err := readYAML([]byte("limits: &limits\n  memory: 256\ncopy: *limits\n")); err != nil {
		t.Fatalf("bounded alias rejected: %v", err)
	}
	policy := domain.OutputComparison{Kind: "tokens"}
	if err := parseComparisonFlags(strings.Fields("float_tolerance 0"), &policy); err != nil || !policy.FloatingPoint {
		t.Fatal("explicit zero tolerance was lost")
	}
}
