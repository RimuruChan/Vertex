package packages

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

func convertedStatementFixture() string {
	return "# Sum\n\nFind **the sum** of *two integers*: $s = a+b$, with $0 \\le a,b \\le 100$.\n\n" +
		"## Input\n\nTwo integers. A literal comparison is `a < b`, and `name_with_underscores` is code.\n\n" +
		"## Output\n\nTheir sum.\n\n" +
		"$$\n\\begin{aligned}\ns &= \\sum_{i=1}^{n} a_i \\\\\n  &= \\frac{2+4}{2}\n\\end{aligned}\n$$\n\n" +
		"| Symbol | Meaning |\n| :--- | ---: |\n| $a$ | First value |\n| $b$ | Second value |\n\n" +
		"3. Read the values.\n4. Print the sum.\n\n> Keep all formatting.\n\n" +
		"```text\n  \\end{quote}\\input{not-an-asset}\n  100% & {braces} # $ _ ^ ~ |\n```\n\n" +
		"![Small diagram](diagram.png)\n"
}
func statementPNG(t *testing.T) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 120, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 120; x++ {
			picture.Set(x, y, color.RGBA{40, 80, 160, 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, picture); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
func TestMarkdownToTeXPreservesSupportedStructure(t *testing.T) {
	ref := domain.Reference(statementPNG(t))
	converted, err := markdownToTeX([]byte(convertedStatementFixture()), "Sum", "statement/problem.en.md", map[string]domain.TreeEntry{"picture": {ID: "picture", Kind: domain.EntryAsset, Path: "statement/diagram.png", Blob: ref}})
	if err != nil {
		t.Fatal(err)
	}
	value := string(converted.data)
	for _, part := range []string{`\problemname{Sum}`, `\textbf{the sum}`, `\emph{two integers}`, `\(s = a+b\)`, `\begin{aligned}`, `\frac{2+4}{2}`, `\begin{tabular}{lr}`, `\setcounter{enumi}{2}`, `\texttt{a \textless{} b}`, `\textbackslash{}end\{quote\}\textbackslash{}input\{not-an-asset\}`, `\includegraphics`} {
		if !strings.Contains(value, part) {
			t.Errorf("missing %q in %s", part, value)
		}
	}
	if strings.Contains(value, `\input{not-an-asset}`) || strings.Contains(value, `\section*{Sum}`) {
		t.Fatal("unsafe code or duplicate title emitted")
	}
	if len(converted.images) != 1 || converted.images["assets/"+ref.SHA256+".png"] != ref {
		t.Fatal("image reference lost")
	}
}

func TestMarkdownToTeXRejectsUnsupportedOrUnsafeMeaning(t *testing.T) {
	for _, value := range []string{
		`$\input{secret}$`, `$\write18{command}$`, `$\csname input\endcsname$`, `$^^5cinput{secret}$`,
		`$\begin{document}x\end{document}$`, `$\frac{1}{2$`, `$x%comment$`, `$\left(x$`,
		"{{nextsample}}", "{{remainingsamples}}", "<script>hidden()</script>", "~~removed text~~", "![secret](../data/secret/1.png)",
		"![remote](https://example.test/private.png)", "[bad](javascript:alert)", "## Sample Input\n\n1 2", "## 样例输出\n\n3",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := markdownToTeX([]byte(value), "Name", "statement/problem.en.md", nil); err == nil {
				t.Fatal("unsafe or unsupported Markdown accepted")
			}
		})
	}
	if _, err := markdownToTeX([]byte("body"), `^^0a\input{secret}`, "statement/problem.en.md", nil); err == nil {
		t.Fatal("title injected a TeX newline through the name comment")
	}
}

func TestStatementImageClassificationKeepsSecretIllustrationsPrivate(t *testing.T) {
	files := kattisFixture("2025-09")
	files["statement/diagram.png"] = string(statementPNG(t))
	files["data/secret/01.png"] = string(statementPNG(t))
	store, _ := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]domain.TreeEntry{}
	for _, entry := range plan.Tree.Entries {
		entries[entry.ID] = entry
		if entry.Path == "statement/diagram.png" && (entry.Kind != domain.EntryAsset || entry.Attributes["purpose"] != "statement-support") {
			t.Fatal("statement image was not identified")
		}
		if entry.Path == "data/secret/01.png" && entry.Kind != domain.EntryResource {
			t.Fatal("secret illustration became public")
		}
	}
	if _, err := markdownToTeX([]byte("![secret](../data/secret/01.png)"), "Name", "statement/problem.en.md", entries); err == nil {
		t.Fatal("private file used as a public image")
	}
}

func TestMarkdownCodeAndEntitiesAreNotInterpretedAsMath(t *testing.T) {
	source := "Escaped \\*text\\*, &amp; and &#36;literal. `$x_1$`\n\n```tex\n$\\input{secret}$\n```\n"
	converted, err := markdownToTeX([]byte(source), "A & B", "statement/problem.en.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`\problemname{A \& B}`, `*text*`, `\&`, `\$literal`, `\texttt{\$x\_1\$}`, `\$\textbackslash{}input\{secret\}\$`} {
		if !strings.Contains(string(converted.data), part) {
			t.Errorf("lost literal %q: %s", part, converted.data)
		}
	}
}

func TestLegacyExportConvertsMarkdownAndPreservesStatementImages(t *testing.T) {
	files := kattisFixture("2025-09")
	files["statement/problem.en.md"] = convertedStatementFixture()
	files["statement/diagram.png"] = string(statementPNG(t))
	store, blobs := testStore(t)
	plan, err := Import(packageZIP(t, files), domain.ImportOptions{}, store)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := plan.Tree.Hash()
	exported, err := Export(plan.Tree, ExportOptions{Format: "kattis-legacy-icpc"}, func(ref domain.BlobRef) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(blobs[ref.SHA256])), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := plan.Tree.Hash()
	if before != after {
		t.Fatal("export mutated the source tree")
	}
	archive, err := zip.NewReader(bytes.NewReader(exported.Data), int64(len(exported.Data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range archive.File {
		names[strings.TrimPrefix(file.Name, strings.TrimSuffix(exported.Filename, ".zip")+"/")] = true
	}
	if !names["problem_statement/problem.en.tex"] || names["problem_statement/problem.en.md"] || !names["problem_statement/diagram.png"] {
		t.Fatalf("wrong exported statement paths: %+v", names)
	}
	ref := domain.Reference(statementPNG(t))
	if !names["problem_statement/assets/"+ref.SHA256+".png"] {
		t.Fatal("converted image target missing")
	}
	converted := false
	for _, issue := range exported.Issues {
		converted = converted || issue.Code == "statement.converted"
	}
	if !converted {
		t.Fatal("conversion was not reported")
	}
}
