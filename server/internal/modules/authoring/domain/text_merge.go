package domain

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// MergeText merges non-overlapping line edits without normalizing bytes. Large
// or ambiguous edits stay conflicts rather than consuming unbounded memory or
// guessing. Import/editor normalization is a separate, explicit operation.
func MergeText(base, local, remote []byte) ([]byte, bool) {
	b, l, r := string(base), string(local), string(remote)
	if merged, ok := mergeValue(b, l, r); ok {
		return []byte(merged), true
	}
	for _, value := range []string{b, l, r} {
		if len(value) > 1<<20 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, false
		}
	}
	baseLines := textLines(b)
	left, ok := lineChanges(baseLines, textLines(l))
	if !ok {
		return nil, false
	}
	right, ok := lineChanges(baseLines, textLines(r))
	if !ok {
		return nil, false
	}
	changes := append(left, right...)
	slices.SortFunc(changes, func(a, b lineChange) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	merged := []lineChange{}
	for _, change := range changes {
		if len(merged) != 0 {
			previous := merged[len(merged)-1]
			if previous.start == change.start && previous.end == change.end && slices.Equal(previous.lines, change.lines) {
				continue
			}
			if change.start < previous.end || change.start == previous.start ||
				(change.start == previous.end && (change.start == change.end || previous.start == previous.end)) {
				return nil, false
			}
		}
		merged = append(merged, change)
	}
	var result strings.Builder
	position := 0
	for _, change := range merged {
		result.WriteString(strings.Join(baseLines[position:change.start], ""))
		result.WriteString(strings.Join(change.lines, ""))
		position = change.end
	}
	result.WriteString(strings.Join(baseLines[position:], ""))
	return []byte(result.String()), true
}

type lineChange struct {
	start, end int
	lines      []string
}

func textLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.SplitAfter(value, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func lineChanges(base, next []string) ([]lineChange, bool) {
	n, m := len(base), len(next)
	const maxCells = 4_000_000
	if n+1 > maxCells/(m+1) {
		return nil, false
	}
	width := m + 1
	lcs := make([]uint32, (n+1)*width)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if base[i] == next[j] {
				lcs[i*width+j] = lcs[(i+1)*width+j+1] + 1
			} else {
				lcs[i*width+j] = max(lcs[(i+1)*width+j], lcs[i*width+j+1])
			}
		}
	}
	changes := []lineChange{}
	var pending *lineChange
	flush := func() {
		if pending != nil {
			changes = append(changes, *pending)
			pending = nil
		}
	}
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && base[i] == next[j] {
			flush()
			i++
			j++
			continue
		}
		if pending == nil {
			pending = &lineChange{start: i, end: i}
		}
		if j < m && (i == n || lcs[i*width+j+1] > lcs[(i+1)*width+j]) {
			pending.lines = append(pending.lines, next[j])
			j++
		} else {
			i++
			pending.end = i
		}
	}
	flush()
	return changes, true
}
