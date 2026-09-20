package builder

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/artifact"
	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/compile"
	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

func TestBuilderRejectsJobsWithoutFrozenInput(t *testing.T) {
	for _, job := range []*Job{nil, {BuildID: "old-inline-job"}} {
		report, err := (&Builder{}).Build(context.Background(), job, nil)
		if err != nil || report == nil || report.Success || report.ErrorMessage == "" {
			t.Fatalf("missing frozen snapshot was accepted: %+v %v", report, err)
		}
	}
}

func TestExpectedSolutionChecksEveryCase(t *testing.T) {
	if expectedSolution([]string{verdict.WA}, []string{verdict.WA, verdict.TLE}) {
		t.Fatal("a wrong-answer reference hid a later timeout")
	}
	if !expectedSolution([]string{verdict.TLE}, []string{verdict.WA, verdict.TLE}) {
		t.Fatal("valid timeout reference rejected")
	}
	if expectedSolution([]string{"Any Rejection"}, []string{verdict.SE}) {
		t.Fatal("judge error counted as a useful rejection")
	}
	if expectedSolution([]string{verdict.AC}, []string{verdict.AC, verdict.WA}) {
		t.Fatal("incorrect accepted reference passed")
	}
}

func TestFrozenContentVerifiesDownloads(t *testing.T) {
	data := []byte("input\n")
	sum := sha256.Sum256(data)
	ref := BlobRef{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
	check := frozenBuild{directory: t.TempDir(), blobs: map[string]string{}, job: &Job{FetchContent: func(context.Context, BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("wrong\n")), nil
	}}}
	if _, err := check.content(context.Background(), ref); err == nil {
		t.Fatal("bad content digest accepted")
	}
}

func TestFrozenBuilderNative(t *testing.T) {
	if os.Getenv("VERTEX_SANDBOX_INTEGRATION") != "1" {
		t.Skip("requires native sandbox in an isolated privileged test container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	if err := os.Mkdir(cache, 0755); err != nil {
		t.Fatal(err)
	}
	client := run.NewClient("/vertex/sandbox", run.DefaultPolicy())
	compiler := compile.NewCompiler(client, cache, root)
	b, err := NewBuilder(client, compiler, root, "/vertex/testlib.h")
	if err != nil {
		t.Fatal(err)
	}
	blobs := map[string][]byte{}
	put := func(text string) BlobRef {
		data := []byte(text)
		sum := sha256.Sum256(data)
		ref := BlobRef{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data))}
		blobs[ref.SHA256] = data
		return ref
	}
	program := func(id, role, lang, protocol, entry string, files map[string]string, expected []string) FrozenProgram {
		value := FrozenProgram{ID: id, EntryPoint: entry, Definition: ProgramDefinition{SchemaVersion: 1, Name: id, Role: role, Language: lang, Protocol: protocol, EntryPoint: entry, ExpectedVerdicts: expected, Arguments: []string{}, Files: []string{}}}
		for name, source := range files {
			value.Files = append(value.Files, FrozenFile{ID: id + "-" + name, Path: name, Blob: put(source)})
			value.Definition.Files = append(value.Definition.Files, id+"-"+name)
		}
		return value
	}
	snapshot := &FrozenSnapshot{SchemaVersion: 1, TreeHash: strings.Repeat("a", 64), DataHash: strings.Repeat("b", 64), PolicyVersion: CheckProtocol, Metadata: FrozenMetadata{SchemaVersion: 1, Title: "Sum", JudgeType: "normal", TimeLimitMs: 1000, MemoryLimitKB: 262144, Comparison: checker.Policy{Kind: "kattis"}, MainSolution: "reference", InputValidators: []string{"validator"}, OutputValidator: "checker", TestOrder: []string{"one"}}}
	snapshot.Programs = []FrozenProgram{
		program("reference", "solution", "cpp", "stdio", "main.cpp", map[string]string{"main.cpp": "#include <iostream>\n#include \"include/add.h\"\nint main(){long long a,b;std::cin>>a>>b;std::cout<<add(a,b)<<'\\n';}\n", "include/add.h": "long long add(long long,long long);\n", "lib/add.cpp": "#include \"../include/add.h\"\nlong long add(long long a,long long b){return a+b;}\n"}, []string{verdict.AC}),
		program("wrong", "solution", "cpp", "stdio", "wrong.cpp", map[string]string{"wrong.cpp": "#include <iostream>\nint main(){std::cout<<0<<'\\n';}\n"}, []string{verdict.WA}),
		program("validator", "input-validator", "python", "kattis", "validate.py", map[string]string{"validate.py": "import sys\na=list(map(int,sys.stdin.read().split()))\nsys.exit(42 if len(a)==2 else 43)\n"}, nil),
		program("checker", "output-validator", "python", "kattis", "check.py", map[string]string{"check.py": "import sys\nfrom pathlib import Path\na,b=map(int,Path(sys.argv[1]).read_text().split())\nassert Path(sys.argv[2]).read_text()=='03\\n', 'imported answer was overwritten'\ntry: ok=int(sys.stdin.read())==a+b\nexcept ValueError: ok=False\nPath(sys.argv[3]+'judgemessage.txt').write_text('private expected sum '+str(a+b))\nif not ok: Path(sys.argv[3]+'teammessage.txt').write_text('check your sum')\nsys.exit(42 if ok else 43)\n"}, nil),
	}
	input, answer := put("1 2\n"), put("03\n")
	test := FrozenTest{ID: "one", Definition: TestDefinition{SchemaVersion: 1, Name: "one", IsSample: true, Points: 0.5}, Input: &input, Answer: &answer}
	test.Definition.Input.Kind = "file"
	test.Definition.Answer.Kind = "file"
	snapshot.Tests = []FrozenTest{test}
	invalidInput, wrongOutput := put("1 2 3\n"), put("4\n")
	snapshot.Validation = []FrozenValidation{
		{ID: "invalid-input", Definition: artifact.ValidationDefinition{SchemaVersion: 1, Name: "extra token", Mode: "invalid_input"}, Input: &invalidInput},
		{ID: "invalid-output", Definition: artifact.ValidationDefinition{SchemaVersion: 1, Name: "wrong sum", Mode: "invalid_output"}, Input: &input, Answer: &answer, Output: &wrongOutput},
		{ID: "valid-output", Definition: artifact.ValidationDefinition{SchemaVersion: 1, Name: "alternate correct spelling", Mode: "valid_output"}, Input: &input, Answer: &answer, Output: &answer},
	}

	job := &Job{Check: snapshot, Limits: Limits{GeneratorTimeMs: 2000, ValidatorTimeMs: 2000, SolutionTimeMs: 2000, CheckerTimeMs: 2000, MemoryLimitKB: 262144}, FetchContent: func(_ context.Context, ref BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}}
	report, err := b.Build(ctx, job, nil)
	if err != nil || !report.Success {
		t.Fatalf("native check failed: %+v %v", report, err)
	}
	if len(report.ToolchainKey) != 64 || len(report.Solutions) != 2 || !report.Solutions[1].Matched || len(report.Solutions[0].Cases) != 1 || report.Tests[0].Points != 0.5 {
		t.Fatalf("incomplete report: %+v", report)
	}
	if len(report.Validation) != 3 {
		t.Fatalf("missing validation outcomes: %+v", report.Validation)
	}
	for _, item := range report.Validation {
		if item.Status != StatusOK {
			t.Fatalf("self-test failed: %+v", item)
		}
	}
	archive, err := zip.NewReader(bytes.NewReader(report.Archive), int64(len(report.Archive)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range archive.File {
		if file.Name == "1.out" {
			reader, _ := file.Open()
			data, _ := io.ReadAll(reader)
			reader.Close()
			if string(data) != "03\n" {
				t.Fatal("imported answer changed")
			}
			found = true
		}
		if file.Name == "artifact.json" {
			reader, _ := file.Open()
			var manifest ArtifactManifest
			if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
				t.Fatal(err)
			}
			reader.Close()
			if len(manifest.Dependencies) != 1 {
				t.Fatal("pinned testlib source missing")
			}
		}
	}
	if !found {
		t.Fatal("answer missing from artifact")
	}

	originalValidator := snapshot.Programs[2]
	for _, scenario := range []struct{ name, source, actual string }{
		{"always accepts", "import sys\nsys.exit(42)\n", "accepted"},
		{"crashes on invalid input", "import sys,os,signal\na=sys.stdin.read().split()\nif len(a)!=2: os.kill(os.getpid(),signal.SIGSEGV)\nsys.exit(42)\n", "error"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			snapshot.Programs[2] = program("validator", "input-validator", "python", "kattis", "validate.py", map[string]string{"validate.py": scenario.source}, nil)
			report, err := b.Build(ctx, job, nil)
			if err != nil || report.Success || len(report.Validation) != 1 || report.Validation[0].Status != StatusFailed || report.Validation[0].Actual != scenario.actual {
				t.Fatalf("bad validator passed: %+v %v", report, err)
			}
		})
	}
	snapshot.Programs[2] = originalValidator
	snapshot.Validation[1].Output = &answer
	report, err = b.Build(ctx, job, nil)
	if err != nil || report.Success || len(report.Validation) != 2 || report.Validation[1].Actual != "accepted" {
		t.Fatalf("incorrect negative expectation passed: %+v %v", report, err)
	}
	snapshot.Validation[1].Output = &wrongOutput
	snapshot.Validation[2].Output = &wrongOutput
	report, err = b.Build(ctx, job, nil)
	if err != nil || report.Success || len(report.Validation) != 3 || report.Validation[2].Actual != "rejected" {
		t.Fatalf("incorrect positive expectation passed: %+v %v", report, err)
	}
}
