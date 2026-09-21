package domain

import "strings"

// Sample is the full input/answer text selected from a verified artifact.
// Report previews are not publication inputs.
type Sample struct {
	Index         int
	Input, Answer string
}
type sampleHeadings struct{ Samples, Sample, Input, Output string }

func headingsFor(language string) sampleHeadings {
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		return sampleHeadings{"样例", "样例", "输入", "输出"}
	}
	return sampleHeadings{"Examples", "Example", "Input", "Output"}
}

// RenderSamples appends reviewed examples to the author's original Markdown.
func RenderSamples(language string, samples []Sample) string {
	if len(samples) == 0 {
		return ""
	}
	sections := headingsFor(language)
	var out strings.Builder
	if len(samples) > 0 {
		out.WriteString("## ")
		out.WriteString(sections.Samples)
		out.WriteString("\n\n")
		for position, sample := range samples {
			if len(samples) > 1 {
				out.WriteString("### ")
				out.WriteString(sections.Sample)
				out.WriteString(" ")
				index := sample.Index
				if index <= 0 {
					index = position + 1
				}
				out.WriteString(itoa(index))
				out.WriteString("\n\n")
			}
			out.WriteString("**")
			out.WriteString(sections.Input)
			out.WriteString("**\n\n")
			writeFence(&out, sample.Input)
			out.WriteString("**")
			out.WriteString(sections.Output)
			out.WriteString("**\n\n")
			writeFence(&out, sample.Answer)
		}
	}

	return strings.TrimRight(out.String(), "\n") + "\n"
}

// writeFence emits a fenced block whose delimiter is always longer than any
// backtick run inside the sample, so test data containing backticks cannot
// break out of the code block.
func writeFence(out *strings.Builder, body string) {
	fence := strings.Repeat("`", longestBacktickRun(body)+1)
	if len(fence) < 3 {
		fence = "```"
	}
	out.WriteString(fence)
	out.WriteString("\n")
	out.WriteString(strings.TrimRight(body, "\n"))
	out.WriteString("\n")
	out.WriteString(fence)
	out.WriteString("\n\n")
}

func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, char := range value {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
