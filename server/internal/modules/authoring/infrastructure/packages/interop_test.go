package packages

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

// Pinned external verifier. Run explicitly on a machine with Podman; ordinary
// unit tests do not download containers or pretend to validate interoperability.
const problemtoolsImage = "docker.io/problemtools/icpc@sha256:36abd98219649fac88dcc22dea6bf59c9e84f11db9385df992382fb531ce7009"

func TestExportWithKattisProblemtools(t *testing.T) {
	if os.Getenv("VERTEX_PACKAGE_INTEROP") != "1" {
		t.Skip("VERTEX_PACKAGE_INTEROP not enabled")
	}
	verifyExternalPackage(t, interopSourceFiles(), "kattis-legacy", false)
}

func TestMarkdownLegacyWithProblemtools(t *testing.T) {
	if os.Getenv("VERTEX_PACKAGE_INTEROP") != "1" {
		t.Skip("VERTEX_PACKAGE_INTEROP not enabled")
	}
	files := interopSourceFiles()
	files["problem.yaml"] = "problem_format_version: 2025-09\nname: Sum\nlicense: cc0\nrights_owner: Vertex test authors\ntype: pass-fail\nlimits:\n  memory: 256\n  time_limit: 1.5\n"
	delete(files, "problem_statement/problem.en.tex")
	files["statement/problem.en.md"] = convertedStatementFixture()
	files["statement/diagram.png"] = string(statementPNG(t))
	files["data/sample/test_group.yaml"] = "output_validator_args: [float_tolerance, '0']\n"
	files["data/secret/test_group.yaml"] = "output_validator_args: [float_tolerance, '0']\n"
	verifyExternalPackage(t, files, "kattis-legacy", true)
}

func interopSourceFiles() map[string]string {
	files := kattisFixture("legacy")
	files["problem.yaml"] = "problem_format_version: legacy\nname: Sum\nlicense: cc0\nrights_owner: Vertex test authors\nvalidator_flags: float_tolerance 0\nlimits:\n  memory: 256\n"
	files["problem_statement/problem.en.tex"] = "\\problemname{Sum}\nCompute the sum of two integers.\n\\section*{Input}\nTwo integers between 0 and 100 separated by a space.\n\\section*{Output}\nTheir sum.\n"
	files["submissions/accepted/reference/main.cpp"] = "#include <iostream>\n#include \"include/add.h\"\nint main(){long long a,b;std::cin>>a>>b;std::cout<<add(a,b)<<'\\n';}\n"
	files["submissions/accepted/reference/include/add.h"] = "inline long long add(long long a,long long b){return a+b;}\n"
	files["submissions/wrong_answer/main.cpp"] = "#include <iostream>\nint main(){std::cout<<0<<'\\n';}\n"
	files["input_validators/validator.cpp"] = "#include <iostream>\n#include <regex>\n#include <string>\nint main(){std::string s;if(!std::getline(std::cin,s)||!std::regex_match(s,std::regex(\"(0|[1-9][0-9]?|100) (0|[1-9][0-9]?|100)\")))return 43;char c;if(std::cin.get(c))return 43;return 42;}\n"
	return files
}

func verifyExternalPackage(t *testing.T, files map[string]string, format string, render bool) {
	t.Helper()
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{TimeLimitMs: 1500}, put)
	if err != nil {
		t.Fatal(err)
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	exported, err := Export(plan.Tree, ExportOptions{Format: format, Identity: "vertex-interop-sum"}, read)
	if err != nil {
		t.Fatal(err)
	}
	verifyExternalExport(t, exported, render)
}

func verifyExternalExport(t *testing.T, exported *ExportResult, render bool) {
	t.Helper()
	root := t.TempDir()
	packageName := strings.TrimSuffix(exported.Filename, ".zip")
	containerDirectory := "/work/" + packageName
	archive, err := zip.NewReader(bytes.NewReader(exported.Data), int64(len(exported.Data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if err := domain.ValidatePackagePath(file.Name); err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(root, filepath.FromSlash(file.Name))
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, data, file.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("podman", "run", "--rm", "--network", "none", "--memory", "2g", "--cpus", "2", "--cap-drop=ALL", "--security-opt=no-new-privileges", "-v", filepath.ToSlash(root)+":/work", problemtoolsImage, "verifyproblem", "-e", "-t", "1.5", containerDirectory)
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		if render {
			diagnostic := exec.Command("podman", "run", "--rm", "--network", "none", "--memory", "2g", "--cpus", "2", "--cap-drop=ALL", "--security-opt=no-new-privileges", "-v", filepath.ToSlash(root)+":/work", problemtoolsImage, "problem2pdf", "-l", "en", "-o", "/tmp/diagnostic.pdf", containerDirectory)
			output, _ := diagnostic.CombinedOutput()
			t.Log(string(output))
		}
		t.Fatalf("external package verifier: %v", err)
	}
	if render {
		command = exec.Command("podman", "run", "--rm", "--network", "none", "--memory", "2g", "--cpus", "2", "--cap-drop=ALL", "--security-opt=no-new-privileges", "-v", filepath.ToSlash(root)+":/work", problemtoolsImage, "/opt/venvs/kattis-problemtools/bin/python", "-c", "from pathlib import Path; from problemtools.problem2pdf import get_parser, convert; options=get_parser().parse_args(['-q','-l','en','-o','/tmp/converted.pdf','"+containerDirectory+"']); assert convert(options); Path('/work/converted.pdf').write_bytes(Path('/tmp/converted.pdf').read_bytes())")
		output, err = command.CombinedOutput()
		t.Log(string(output))
		if err != nil {
			t.Fatalf("converted statement did not compile: %v", err)
		}
		pdf, err := os.ReadFile(filepath.Join(root, "converted.pdf"))
		if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
			t.Fatalf("missing rendered statement: %v", err)
		}
		if target := os.Getenv("VERTEX_RENDERED_STATEMENT"); target != "" {
			if err := os.WriteFile(target, pdf, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}

}
