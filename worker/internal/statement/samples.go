package statement

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Sample struct{ Input, Answer string }
type Options struct {
	Title   string
	Samples []Sample
}

var literal = strings.NewReplacer(`\`, `\textbackslash{}`, `{`, `\{`, `}`, `\}`, `$`, `\$`, `&`, `\&`, `#`, `\#`, `%`, `\%`, `_`, `\_`, `~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`)

// Only bounded text previews are staged. Binary/large examples remain available
// as original downloads in the published view; their bytes are never TeX code.
func prepareSamples(work string, options Options) (string, map[string]string, error) {
	if len(options.Samples) > 500 {
		return "", nil, fmt.Errorf("too many statement samples")
	}
	var commands strings.Builder
	title := options.Title
	if title == "" {
		title = "Problem"
	}
	fmt.Fprintf(&commands, "\\def\\vertextitle{%s}\n\\def\\vertexsamplecount{%d}\n", literal.Replace(title), len(options.Samples))
	files := map[string]string{}
	for index, sample := range options.Samples {
		fmt.Fprintf(&commands, "\\expandafter\\def\\csname vertexsample%d\\endcsname{\\section*{Sample %d}\n", index+1, index+1)
		for part, file := range []string{sample.Input, sample.Answer} {
			label, suffix := "Input", "in"
			if part == 1 {
				label, suffix = "Output", "ans"
			}
			fmt.Fprintf(&commands, "\\noindent\\textbf{%s}\\par\n", label)
			body, err := samplePreview(file)
			if err != nil {
				return "", nil, err
			}
			if body == nil {
				commands.WriteString("\\noindent\\textit{Large or binary sample; download the original file from the problem page.}\\par\n")
				continue
			}
			name := fmt.Sprintf("__vertex_sample_%d.%s", index+1, suffix)
			target := filepath.Join(work, name)
			if err := os.WriteFile(target, body, 0600); err != nil {
				return "", nil, err
			}
			files[name] = target
			fmt.Fprintf(&commands, "\\VerbatimInput[fontsize=\\small,breaklines,breakanywhere,frame=single]{%s}\n", name)
		}
		commands.WriteString("}\n")
	}
	return commands.String(), files, nil
}

func samplePreview(name string) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil {
		return nil, err
	}
	if len(data) > 8192 || !utf8.Valid(data) {
		return nil, nil
	}
	for _, value := range data {
		if value < 32 && value != '\n' && value != '\r' && value != '\t' || value == 127 {
			return nil, nil
		}
	}
	return data, nil
}
