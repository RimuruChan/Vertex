package domain

import (
	"slices"
	"testing"
)

func TestTransformTagsPreservesFirstPosition(t *testing.T) {
	for _, test := range []struct {
		name           string
		tags, expected []string
		remove         bool
	}{
		{"rename", []string{"a", "old", "z"}, []string{"a", "new", "z"}, false},
		{"merge", []string{"old", "a", "new", "a"}, []string{"new", "a"}, false},
		{"delete", []string{"a", "old", "z"}, []string{"a", "z"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := slices.Clone(test.tags)
			got := TransformTags(test.tags, "old", "new", test.remove)
			if !slices.Equal(got, test.expected) {
				t.Fatalf("got %v, want %v", got, test.expected)
			}
			if !slices.Equal(original, test.tags) {
				t.Fatal("input snapshot was modified")
			}
		})
	}
}
