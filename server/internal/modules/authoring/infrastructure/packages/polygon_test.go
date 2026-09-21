package packages

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func polygonExecutionFixture(t *testing.T) map[string]string {
	files := polygonFixture()
	files["tests/01.a"] = "3\n"
	files["problem.xml"] = strings.Replace(files["problem.xml"], "</solutions>", `<solution tag="wrong-answer"><source path="solutions/wrong.cpp" type="cpp.g++17"/></solution></solutions>`, 1)
	files["solutions/wrong.cpp"] = "#include <iostream>\nint main(){std::cout<<0<<'\\n';}\n"
	files["statements/english/diagram.png"] = string(statementPNG(t))
	files["statements/english/problem.tex"] = `\begin{problem}{Sum}{standard input}{standard output}{1.5 seconds}{256 megabytes}
Read two integers and print their sum.
\InputFile
Two integers between 0 and 100, separated by a space.
\OutputFile
Print the sum.
\includegraphics[width=.4\textwidth]{diagram.png}
\Examples
\begin{example}
\exmp{1 2
}{3
}%
\end{example}
\Note
The two numbers are added.
\end{problem}
`
	return files
}

func TestPolygonStandardLayoutRoundTrip(t *testing.T) {
	files := polygonExecutionFixture(t)
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	before, _ := plan.Tree.Hash()
	exported, err := Export(plan.Tree, ExportOptions{Format: "kattis-legacy", Identity: "polygon-sum"}, read)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := plan.Tree.Hash()
	if before != after {
		t.Fatal("export modified Polygon source")
	}
	archive, err := readArchive(exported.Data)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.selectRoot(); err != nil {
		t.Fatal(err)
	}
	source, err := archive.read("problem_statement/polygon-en/problem.tex", 1<<20)
	if err != nil || string(source) != files["statements/english/problem.tex"] {
		t.Fatal("original Polygon TeX was rewritten or lost")
	}
	if !archive.has("problem_statement/polygon-en/diagram.png") {
		t.Fatal("relative image layout was lost")
	}
	second, err := Import(exported.Data, domain.ImportOptions{TimeLimitMs: 1500}, put)
	if err != nil {
		t.Fatal(err)
	}
	report, snapshot, err := domain.InspectMaterials(second.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil || !report.CanBuild || len(snapshot.Statements) != 1 || len(snapshot.Statements[0].Files) != 2 {
		t.Fatalf("wrapped TeX dependencies not available: %+v %v", report, err)
	}
	if target := os.Getenv("VERTEX_POLYGON_ARCHIVE"); target != "" {
		if err := os.WriteFile(target, packageZIP(t, files), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if target := os.Getenv("VERTEX_POLYGON_KATTIS_ARCHIVE"); target != "" {
		if err := os.WriteFile(target, exported.Data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPolygonLegacyWithProblemtools(t *testing.T) {
	if os.Getenv("VERTEX_PACKAGE_INTEROP") != "1" {
		t.Skip("explicit external verifier required")
	}
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, polygonExecutionFixture(t)), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	// This synthetic fixture is authored here; provide its license explicitly.
	for index, entry := range plan.Tree.Entries {
		if entry.Kind == domain.EntryMetadata {
			view, err := domain.DecodeMaterial(entry, blobs[entry.Blob.SHA256])
			if err != nil {
				t.Fatal(err)
			}
			view.Metadata.License = "cc0"
			view.Metadata.RightsOwner = "Vertex test authors"
			data, _ := json.Marshal(view.Metadata)
			plan.Tree.Entries[index].Blob, err = put(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	exported, err := Export(plan.Tree, ExportOptions{Format: "kattis-legacy", Identity: "polygon-sum"}, func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	verifyExternalExport(t, exported, true)
}

func polygonFixture() map[string]string {
	return map[string]string{
		"problem.xml": `<problem short-name="sum"><names><name language="english" value="Sum"/></names>
<statements><statement language="english" type="application/x-tex" path="statements/english/problem.tex"/></statements>
<judging input-file="" output-file=""><testset name="tests"><time-limit>1500</time-limit><memory-limit>268435456</memory-limit><test-count>2</test-count><input-path-pattern>tests/%02d</input-path-pattern><answer-path-pattern>tests/%02d.a</answer-path-pattern><tests><test method="manual" sample="true"/><test method="generated" cmd="gen 1"/></tests></testset></judging>
<assets><checker type="testlib"><source path="files/check.cpp" type="cpp.g++17"/></checker><validator><source path="files/validate.cpp" type="cpp.g++17"/></validator><solutions><solution tag="main"><source path="solutions/main.cpp" type="cpp.g++17"/></solution></solutions></assets></problem>`,
		"statements/english/problem.tex": `\begin{problem}{Sum}{standard input}{standard output}{1.5 seconds}{256 megabytes}Read two integers.\end{problem}`,
		"files/check.cpp": `#include "testlib.h"
int main(int argc,char**argv){registerTestlibCmd(argc,argv);long long a=ans.readLong(),b=ouf.readLong();if(a!=b)quitf(_wa,"different");quitf(_ok,"ok");}`,
		"files/validate.cpp": `#include "testlib.h"
int main(int argc,char**argv){registerValidation(argc,argv);inf.readInt();inf.readSpace();inf.readInt();inf.readEoln();inf.readEof();}`,
		"solutions/main.cpp": "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}\n",
		"tests/01":           "1 2\n", "tests/01.a": "03\n", "tests/02": "2 5\n", "tests/02.a": "7\n",
	}
}

func TestPolygonMaterializedPackage(t *testing.T) {
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, polygonFixture()), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	inspection, snapshot, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil || !inspection.CanBuild {
		t.Fatalf("inspection: %+v %v", inspection, err)
	}
	meta := snapshot.Metadata
	if len(snapshot.Statements) != 1 || snapshot.Statements[0].Dialect != "polygon" {
		t.Fatalf("Polygon dialect was not sealed into check: %+v", snapshot.Statements)
	}
	if meta.Title != "Sum" || meta.TimeLimitMs != 1500 || meta.MemoryLimitKB != 262144 || meta.ResourceMode != "exact" || meta.Comparison.Kind != "testlib" || len(meta.Requirements) != 0 {
		t.Fatalf("metadata: %+v", meta)
	}
	if len(snapshot.Tests) != 2 || !snapshot.Tests[0].Definition.IsSample || snapshot.Tests[1].Definition.Input.Kind != "file" {
		t.Fatalf("test mapping: %+v", snapshot.Tests)
	}
	foundAnswer, foundOriginal := false, false
	for _, entry := range plan.Tree.Entries {
		if entry.Path == "tests/01.a" {
			foundAnswer = string(blobs[entry.Blob.SHA256]) == "03\n"
		}
		if entry.Path == "problem.xml" {
			foundOriginal = entry.Kind == domain.EntryResource
		}
	}
	if !foundAnswer || !foundOriginal {
		t.Fatal("original answer or recipe lost")
	}
	for _, entry := range plan.Tree.Entries {
		if entry.Kind != domain.EntryProgram {
			continue
		}
		var program domain.ProgramMaterial
		if err := json.Unmarshal(blobs[entry.Blob.SHA256], &program); err != nil {
			t.Fatal(err)
		}
		if program.Role != "solution" && program.Protocol != "testlib" {
			t.Fatal("checker/validator protocol was not preserved")
		}
	}
}

func TestPolygonStatementResourcesAreScoped(t *testing.T) {
	files := polygonFixture()
	files["statements/english/diagram.png"] = string(statementPNG(t))
	files["data/secret/private.png"] = string(statementPNG(t))
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	_, snapshot, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Statements) != 1 || len(snapshot.Statements[0].Files) != 1 || snapshot.Statements[0].Files[0].Path != "statements/english/diagram.png" {
		t.Fatalf("private or missing statement resource: %+v", snapshot.Statements)
	}
	for _, entry := range plan.Tree.Entries {
		if entry.Path == "data/secret/private.png" && entry.Kind != domain.EntryResource {
			t.Fatal("secret illustration became public")
		}
	}
}

func TestPolygonRejectsUnsafeOrIncompletePackage(t *testing.T) {
	for _, replacement := range []string{"tests/../../%02d", "tests/%99d", "tests/%s", "tests/%02d/%02d"} {
		if _, err := expandPolygonPattern(replacement, 1); err == nil {
			t.Fatalf("unsafe pattern accepted: %s", replacement)
		}
	}
	if _, err := readXML([]byte(`<!DOCTYPE problem [<!ENTITY secret SYSTEM "file:///etc/passwd">]><problem/>`)); err == nil {
		t.Fatal("DTD accepted")
	}
	files := polygonFixture()
	delete(files, "tests/02.a")
	put, _ := testStore(t)
	if _, err := Import(packageZIP(t, files), domain.ImportOptions{}, put); err == nil {
		t.Fatal("missing generated answer accepted")
	}
	files = polygonFixture()
	files["problem.xml"] = strings.ReplaceAll(files["problem.xml"], "cpp.g++17", "cpp.g++20")
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, put)
	if err != nil {
		t.Fatal(err)
	}
	inspection, _, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
	if err != nil || inspection.CanBuild {
		t.Fatalf("unavailable toolchain not blocked: %+v %v", inspection, err)
	}
}
