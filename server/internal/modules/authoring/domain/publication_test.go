package domain

import (
	"errors"
	"testing"
)

func TestPublicationReviewBoundary(t *testing.T) {
	input := PublishInput{Revision: 3, ArtifactVersion: 2}
	candidate := PublicationCandidate{Version: 2, DataRevision: 2, CaseCount: 1, StoragePath: "sha/data", SHA256: "sha"}
	for _, test := range []struct {
		name                   string
		revision, dataRevision int
		candidate              PublicationCandidate
		want                   error
	}{
		{"reviewed", 3, 2, candidate, nil},
		{"edited workspace", 4, 2, candidate, ErrRevisionConflict},
		{"stale candidate", 3, 3, candidate, ErrRevisionConflict},
		{"unusable artifact", 3, 2, PublicationCandidate{Version: 2, DataRevision: 2}, ErrNotPublished},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePublication(input, test.revision, test.dataRevision, test.candidate); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestPublicationLanguageBoundary(t *testing.T) {
	if _, _, err := PublicationText("题目", "题面", "zh", "en", nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("missing requested translation accepted")
	}
	if _, _, err := PublicationText("题目", " ", "zh", "zh", nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("empty release accepted")
	}
	title, body, err := PublicationText("题目", "题面", "zh", "zh", nil, nil)
	if err != nil || title != "题目" || body != "题面" {
		t.Fatalf("default statement lost: %q %q %v", title, body, err)
	}
}
