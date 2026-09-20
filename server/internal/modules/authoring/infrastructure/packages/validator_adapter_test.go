package packages

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

const adapterInputSource = `#include "testlib.h"
int main(int argc,char**argv){registerValidation(argc,argv);inf.readInt(0,100);inf.readSpace();inf.readInt(0,100);inf.readEoln();inf.readEof();}
`
const adapterOutputSource = `#include "testlib.h"
int main(int argc,char**argv){registerTestlibCmd(argc,argv);long long a=std::stoll(ans.readToken()),b=ouf.readLong();if(a!=b)quitf(_wa,"private expected %lld",a);quitf(_ok,"ok");}
`

func adapterExport(t *testing.T, kind, format string) *ExportResult {
	t.Helper()
	input := interopSourceFiles()
	input["data/sample/01.ans"] = "3\n"
	input["input_validators/validator.cpp"] = adapterInputSource
	if kind == "testlib" {
		input["problem.yaml"] = "problem_format_version: legacy\nname: Sum\nlicense: cc0\nrights_owner: Vertex test authors\nvalidation: custom\nlimits:\n  memory: 256\n"
		input["output_validators/checker.cpp"] = adapterOutputSource
	}
	put, blobs := testStore(t)
	plan, err := Import(packageZIP(t, input), domain.ImportOptions{TimeLimitMs: 1500}, put)
	if err != nil {
		t.Fatal(err)
	}
	for index, entry := range plan.Tree.Entries {
		if entry.Kind != domain.EntryMetadata && entry.Kind != domain.EntryProgram {
			continue
		}
		view, err := domain.DecodeMaterial(entry, blobs[entry.Blob.SHA256])
		if err != nil {
			t.Fatal(err)
		}
		var updated any
		if view.Metadata != nil {
			view.Metadata.Comparison = domain.OutputComparison{Kind: kind}
			updated = view.Metadata
		} else {
			if view.Program.Role == "input-validator" || view.Program.Role == "output-validator" {
				view.Program.Protocol = "testlib"
			}
			updated = view.Program
		}
		data, _ := json.Marshal(updated)
		plan.Tree.Entries[index].Blob, err = put(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
	}
	read := func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	}
	exported, err := Export(plan.Tree, ExportOptions{Format: format, Identity: "adapter-test"}, read)
	if err != nil {
		t.Fatal(err)
	}
	return exported
}

func TestValidatorAdapterArchive(t *testing.T) {
	if domain.Digest(exportTestlib) != exportTestlibSHA256 {
		t.Fatal("embedded testlib differs from worker pin")
	}
	for _, format := range []string{"kattis-legacy", "kattis-legacy-icpc", "kattis-2025-09", "domjudge"} {
		t.Run(format, func(t *testing.T) {
			exported := adapterExport(t, "testlib", format)
			if target := os.Getenv("VERTEX_ADAPTER_ARCHIVE"); target != "" && format == "kattis-legacy" {
				if err := os.WriteFile(target, exported.Data, 0644); err != nil {
					t.Fatal(err)
				}
			}
			archive, err := zip.NewReader(bytes.NewReader(exported.Data), int64(len(exported.Data)))
			if err != nil {
				t.Fatal(err)
			}
			wrappers, originals, headers := 0, 0, 0
			for _, file := range archive.File {
				r, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(r)
				r.Close()
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasSuffix(file.Name, "/run") {
					wrappers++
					if file.Mode().Perm()&0111 == 0 {
						t.Fatal("run is not executable")
					}
				}
				if strings.HasSuffix(file.Name, ".cpp") && (string(data) == adapterInputSource || string(data) == adapterOutputSource) {
					originals++
					if string(data) != adapterInputSource && string(data) != adapterOutputSource {
						t.Fatal("original source changed")
					}
				}
				if strings.HasSuffix(file.Name, "/testlib.h") {
					headers++
					if domain.Digest(data) != exportTestlibSHA256 {
						t.Fatal("unpinned testlib exported")
					}
				}
			}
			if wrappers != 2 || originals != 2 || headers < 2 {
				t.Fatalf("incomplete adapter package: %d/%d/%d", wrappers, originals, headers)
			}
			put, blobs := testStore(t)
			plan, err := Import(exported.Data, domain.ImportOptions{TimeLimitMs: 1500}, put)
			if err != nil {
				t.Fatal(err)
			}
			inspection, _, err := domain.InspectMaterials(plan.Tree, func(ref domain.BlobRef) ([]byte, error) { return blobs[ref.SHA256], nil })
			if err != nil || !inspection.CanBuild {
				t.Fatalf("exported adapters cannot be reimported: %+v %v", inspection, err)
			}
		})
	}
}

func TestValidatorAdaptersWithProblemtools(t *testing.T) {
	if os.Getenv("VERTEX_PACKAGE_INTEROP") != "1" {
		t.Skip("explicit external verifier required")
	}
	for _, kind := range []string{"testlib", "exact"} {
		t.Run(kind, func(t *testing.T) { verifyExternalExport(t, adapterExport(t, kind, "kattis-legacy"), false) })
	}
}

func TestModifiedAdapterIsNotTrustedOnImport(t *testing.T) {
	exported := adapterExport(t, "testlib", "kattis-legacy")
	archive, err := zip.NewReader(bytes.NewReader(exported.Data), int64(len(exported.Data)))
	if err != nil {
		t.Fatal(err)
	}
	original := map[string]string{}
	marker := ""
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		original[file.Name] = string(data)
		if strings.HasPrefix(strings.TrimPrefix(file.Name, strings.TrimSuffix(exported.Filename, ".zip")+"/"), "output_validators/") && strings.HasSuffix(file.Name, "/vertex-adapter.json") {
			marker = strings.TrimSuffix(file.Name, "/vertex-adapter.json")
		}
	}
	if marker == "" {
		t.Fatal("adapter marker absent")
	}
	for _, mode := range []string{"script", "header", "extra", "trailing-json"} {
		t.Run(mode, func(t *testing.T) {
			files := map[string]string{}
			for name, data := range original {
				files[name] = data
			}
			switch mode {
			case "script":
				files[marker+"/run"] += "\nprint('changed')\n"
			case "header":
				files[marker+"/testlib.h"] += "\n#define quitf(...) exit(0)\n"
			case "extra":
				files[marker+"/extra.h"] = "#define altered 1\n"
			case "trailing-json":
				files[marker+"/vertex-adapter.json"] += "{}"
			}
			put, _ := testStore(t)
			if _, err := Import(packageZIP(t, files), domain.ImportOptions{TimeLimitMs: 1500}, put); err == nil {
				t.Fatal("changed adapter imported as unchanged protocol")
			}
		})
	}
}

func TestValidatorAdapterExitSemantics(t *testing.T) {
	if os.Getenv("VERTEX_PACKAGE_INTEROP") != "1" {
		t.Skip("explicit native compiler verification required")
	}
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("testlib.h", string(exportTestlib))
	write("original.cpp", `#include "testlib.h"
int main(int argc,char**argv){registerTestlibCmd(argc,argv);int mode=inf.readInt();if(mode==1)quitf(_fail,"jury fault");if(mode==2){ouf.readLong();quitp(0.5,"partial");}if(mode==3)return 0;if(mode==4)abort();if(ouf.readLong()!=std::stoll(ans.readToken()))quitf(_wa,"private answer");quitf(_ok,"ok");}
`)
	write("run", testlibAdapter)
	config := adapterConfig{SchemaVersion: 1, Role: "output-validator", EntryPoint: "original.cpp", Files: []string{"original.cpp"}, Arguments: []string{}}
	configBytes, _ := json.Marshal(config)
	write("vertex-adapter.json", string(configBytes))
	build, _ := adapterBuild(config)
	write("build", build)
	write("exact.cpp", string(exactValidator))
	// Compile the actual exported adapter, then exercise bytes and exit codes in
	// an isolated external toolchain. No mock exit-code lookup stands in for it.
	write("verify.py", `import pathlib, subprocess, tempfile, shutil, sys
p=pathlib.Path('/work')
shutil.copytree('/work','/tmp/adapter')
subprocess.run(['/bin/sh','build'],cwd='/tmp/adapter',check=True)
subprocess.run(['g++','-std=c++17','-O2','/work/exact.cpp','-o','/tmp/exact'],check=True)
with tempfile.TemporaryDirectory() as d:
    d=pathlib.Path(d); inp=d/'in'; ans=d/'ans'
    ans.write_bytes(b'03\n')
    for mode,output,expected in [(0,b'3\n',42),(0,b'4\n',43),(0,b'',43),(1,b'3\n',44),(2,b'3\n',44),(3,b'3\n',44),(4,b'3\n',44)]:
        inp.write_text(str(mode)+'\n')
        result=subprocess.run([sys.executable,'/tmp/adapter/run',str(inp),str(ans),str(d)],input=output,timeout=5)
        assert result.returncode==expected,(mode,result.returncode,expected,(d/"judgemessage.txt").read_text())
        assert not (d/'teammessage.txt').exists()
        assert not list(d.glob('vertex-output-*'))
    for answer,output,expected in [(b'a\x00b\n',b'a\x00b\n',42),(b'a\n',b'a',43),(b'a\n',b'A\n',43),(b'a b\n',b'a  b\n',43),(b'',b'',42)]:
        ans.write_bytes(answer)
        result=subprocess.run(['/tmp/exact',str(inp),str(ans),str(d)],input=output,timeout=5)
        assert result.returncode==expected,(answer,output,result.returncode)
print('native adapter byte/exit/feedback checks passed')
`)
	command := exec.Command("podman", "run", "--rm", "--network", "none", "--memory", "2g", "--cpus", "2", "--cap-drop=ALL", "--security-opt=no-new-privileges", "-v", filepath.ToSlash(root)+":/work:ro", problemtoolsImage, "python3", "/work/verify.py")
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
