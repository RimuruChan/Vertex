package domain

import "testing"

func TestContestProblemLabels(t *testing.T) {
	for _, value := range []string{"A", "A1", "ABCDEFGH"} {
		if _, err := ParseProblemLabel(value); err != nil {
			t.Errorf("valid label %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "1", "A-1", "ABCDEFGHI", "00000000-0000-4000-8000-000000000001"} {
		if _, err := ParseProblemLabel(value); err == nil {
			t.Errorf("invalid label accepted: %q", value)
		}
	}
}
