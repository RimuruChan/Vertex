package domain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type sampleMarker struct {
	start, end int
	remaining  bool
}

// PlaceSamples replaces commands in ordinary Markdown text, retaining code
// spans, code blocks and escaped braces as literal source. Inserted sample
// bytes are never scanned again for commands.
func PlaceSamples(source, language string, samples []Sample, inline bool) (string, error) {
	data := []byte(source)
	tree := goldmark.DefaultParser().Parse(text.NewReader(data))
	markers := []sampleMarker{}
	if err := ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node.Kind() == ast.KindCodeSpan {
			return ast.WalkSkipChildren, nil
		}
		part, ok := node.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		for _, command := range []string{"{{nextsample}}", "{{remainingsamples}}"} {
			body := part.Segment.Value(data)
			for offset := 0; offset < len(body); {
				index := strings.Index(string(body[offset:]), command)
				if index < 0 {
					break
				}
				start := part.Segment.Start + offset + index
				slashes := 0
				for i := start - 1; i >= 0 && data[i] == '\\'; i-- {
					slashes++
				}
				if slashes%2 == 0 {
					markers = append(markers, sampleMarker{start: start, end: start + len(command), remaining: command == "{{remainingsamples}}"})
				}
				offset += index + len(command)
			}
		}
		return ast.WalkContinue, nil
	}); err != nil {
		return "", err
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].start < markers[j].start })
	var out strings.Builder
	position, used := 0, 0
	for _, marker := range markers {
		out.WriteString(source[position:marker.start])
		out.WriteString("\n\n")
		count := 1
		if marker.remaining {
			count = len(samples) - used
		}
		if used+count > len(samples) {
			return "", InvalidInput("题面的 nextsample 超过可用样例数量")
		}
		for _, sample := range samples[used : used+count] {
			if inline {
				fragment := RenderSamples(language, []Sample{sample})
				if len(samples) > 1 {
					fragment = strings.Replace(fragment, "## "+headingsFor(language).Samples, fmt.Sprintf("### %s %d", headingsFor(language).Sample, sample.Index), 1)
				}
				out.WriteString(fragment)
			} else {
				fmt.Fprintf(&out, "[%s %d](#sample-%d)\n", headingsFor(language).Sample, sample.Index, sample.Index)
			}
			out.WriteString("\n")
		}
		used += count
		position = marker.end
	}
	out.WriteString(source[position:])
	if inline && used < len(samples) {
		out.WriteString("\n\n")
		out.WriteString(RenderSamples(language, samples[used:]))
	}
	return out.String(), nil
}
