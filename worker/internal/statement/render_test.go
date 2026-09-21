package statement

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/run"
)

func TestRejectMarkupInTeXPaths(t *testing.T) {
	for _, name := range []string{"../secret.tex", "statement/{evil}.tex", "statement/a^^5c.tex", "statement/%comment.tex", "statement/" + wrapperName} {
		if validateInputs(name, map[string]string{name: "unused"}) == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	if err := validateInputs("statement/problem.en.tex", map[string]string{"statement/problem.en.tex": "source", "statement/images/diagram.png": "image"}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderNative(t *testing.T) {
	if os.Getenv("VERTEX_TEX_INTEGRATION") != "1" {
		t.Skip("requires configured TeX and native sandbox in isolated test container")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	key, err := Fingerprint(ctx)
	if err != nil || len(key) != 64 {
		t.Fatalf("fingerprint: %s %v", key, err)
	}
	client := run.NewClient("/vertex/sandbox", run.DefaultPolicy())
	illustration := filepath.Join(root, "image.png")
	f, err := os.Create(illustration)
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 120, 30))
	for x := 0; x < 120; x++ {
		for y := 0; y < 30; y++ {
			picture.Set(x, y, color.RGBA{R: 80, G: 120, B: 200, A: 255})
		}
	}
	if err := png.Encode(f, picture); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.tex")
	outside := filepath.Join(root, "private.txt")
	if err := os.WriteFile(outside, []byte("SECRET-NOT-STAGED"), 0600); err != nil {
		t.Fatal(err)
	}
	contents := `\problemname{涓ゆ暟涔嬪拰 / Sum}
Find $a+b$; $\sum_{i=1}^n i = \frac{n(n+1)}2$.
\section*{杈撳叆鏍煎紡}璇诲彇涓や釜鏁存暟銆俓section*{Output}Print the sum.
\includegraphics[width=.4\textwidth]{images/diagram.png}
\begin{tabular}{ll}A&B\\1&2\end{tabular}
\newread\secret\openin\secret=` + filepath.ToSlash(outside) + `\relax
\ifeof\secret\else\errmessage{unstaged file was readable}\fi\closein\secret
\ifnum\shellescape=0\relax\else\errmessage{shell escape must be disabled}\fi
`
	if err := os.WriteFile(source, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "rendered.pdf")
	if err := Render(ctx, client, root, "statement/problem.zh.tex", map[string]string{"statement/problem.zh.tex": source, "statement/images/diagram.png": illustration}, destination, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || !strings.HasPrefix(string(data), "%PDF-") || len(data) < 1000 {
		t.Fatalf("missing rendered PDF: %v", err)
	}
	if target := os.Getenv("VERTEX_TEX_QA_PDF"); target != "" {
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	sampleInput, sampleAnswer := filepath.Join(root, "sample.in"), filepath.Join(root, "sample.ans")
	if err := os.WriteFile(sampleInput, []byte("1 2\n\\input{/etc/passwd}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sampleAnswer, []byte("3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	standardSource := `\problemname
\newcount\seensamples
\expandafter\let\expandafter\savedfirst\csname vertexsample1\endcsname
\expandafter\let\expandafter\savedsecond\csname vertexsample2\endcsname
\expandafter\def\csname vertexsample1\endcsname{\global\advance\seensamples by 1\relax\savedfirst}
\expandafter\def\csname vertexsample2\endcsname{\global\advance\seensamples by 1\relax\savedsecond}
\begin{Input}Two integers.\end{Input}
\begin{Output}Their sum.\end{Output}
Before the first example.
\nextsample
\ifnum\seensamples=1\relax\else\errmessage{first sample was not rendered at its marker}\fi
After the first example, before the remaining example.
\remainingsamples
\ifnum\seensamples=2\relax\else\errmessage{remaining sample was not rendered at its marker}\fi
After all examples.
`
	if err := os.WriteFile(source, []byte(standardSource), 0600); err != nil {
		t.Fatal(err)
	}
	standardPDF := filepath.Join(root, "standard.pdf")
	options := Options{Title: "Metadata title & safety", Samples: []Sample{{Input: sampleInput, Answer: sampleAnswer}, {Input: sampleInput, Answer: sampleAnswer}}}
	if err := Render(ctx, client, root, "statement/problem.en.tex", map[string]string{"statement/problem.en.tex": source}, standardPDF, "", options); err != nil {
		t.Fatal(err)
	}
	if target := os.Getenv("VERTEX_STANDARD_QA_PDF"); target != "" {
		data, err := os.ReadFile(standardPDF)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(source, []byte(standardSource+`\nextsample`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Render(ctx, client, root, "statement/problem.en.tex", map[string]string{"statement/problem.en.tex": source}, filepath.Join(root, "overflow.pdf"), "", options); err == nil || !strings.Contains(err.Error(), "No remaining sample") {
		t.Fatalf("missing sample was not rejected: %v", err)
	}
	polygon := `\begin{problem}{Polygon sum}{standard input}{standard output}{1 second}{256 megabytes}
Read several integers and print their sum.
\InputFile
An integer $n$, followed by $n$ integers.
\OutputFile
Print the sum.
\includegraphics[width=.4\textwidth]{images/diagram.png}
\Examples
\begin{example}
\exmp{3
1 2 3
}{6
}%
\end{example}
\Note
The sample has three numbers.
\end{problem}
`
	if err := os.WriteFile(source, []byte(polygon), 0600); err != nil {
		t.Fatal(err)
	}
	polygonPDF := filepath.Join(root, "polygon.pdf")
	if err := Render(ctx, client, root, "statements/english/problem.tex", map[string]string{"statements/english/problem.tex": source, "statements/english/images/diagram.png": illustration}, polygonPDF, "polygon"); err != nil {
		t.Fatal(err)
	}
	if target := os.Getenv("VERTEX_POLYGON_QA_PDF"); target != "" {
		data, err := os.ReadFile(polygonPDF)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(source, []byte(`\loop\iftrue\repeat`), 0600); err != nil {
		t.Fatal(err)
	}
	short, stop := context.WithTimeout(ctx, 2*time.Second)
	defer stop()
	started := time.Now()
	if err := Render(short, client, root, "statement/problem.tex", map[string]string{"statement/problem.tex": source}, filepath.Join(root, "loop.pdf"), ""); err == nil {
		t.Fatal("runaway TeX succeeded")
	}
	if time.Since(started) > 10*time.Second {
		t.Fatal("cancelled renderer did not terminate promptly")
	}
}
