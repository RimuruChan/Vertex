package domain

import (
	"strings"
	"testing"
)

func TestMarkdownSampleCommandsPreserveLiteralTextAndSampleBytes(t *testing.T) {
	samples := []Sample{{Index: 1, Input: "{{nextsample}}\n", Answer: "3\n"}, {Index: 2, Input: "2 5\n", Answer: "7\n"}}
	source := "Intro\n\n`{{nextsample}}`\n\n```\n{{nextsample}}\n```\n\n\\{{nextsample}}\n\n{{nextsample}}\n\nBetween\n\n{{remainingsamples}}\n\nEnd"
	result, err := PlaceSamples(source, "en", samples, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "`{{nextsample}}`") || !strings.Contains(result, "\\{{nextsample}}") || !strings.Contains(result, "```\n{{nextsample}}\n```") {
		t.Fatal("literal command altered")
	}
	first, between, second, end := strings.Index(result, "### Example 1"), strings.Index(result, "Between"), strings.Index(result, "### Example 2"), strings.Index(result, "End")
	if first < 0 || between < first || second < between || end < second {
		t.Fatalf("sample positions changed: %s", result)
	}
	if _, err := PlaceSamples("{{nextsample}}", "en", nil, true); err == nil {
		t.Fatal("exhausted nextsample accepted")
	}
	if value, err := PlaceSamples("{{remainingsamples}}", "en", nil, true); err != nil || strings.TrimSpace(value) != "" {
		t.Fatalf("empty remainder: %q %v", value, err)
	}
	if value, err := PlaceSamples("Intro", "en", samples, true); err != nil || !strings.Contains(value, "### Example 2") {
		t.Fatalf("unused samples not appended: %q %v", value, err)
	}
}

func TestLargeMarkdownSamplesLinkToPublicDownloads(t *testing.T) {
	value, err := PlaceSamples("Before\n\n{{nextsample}}\n\nAfter", "zh", []Sample{{Index: 1}}, false)
	if err != nil || !strings.Contains(value, "[样例 1](#sample-1)") {
		t.Fatalf("missing sample link: %q %v", value, err)
	}
}
