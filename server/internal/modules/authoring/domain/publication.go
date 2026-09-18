package domain

import "strings"

// PublicationCandidate is the reviewed artifact, independent of SQL storage.
type PublicationCandidate struct {
	Version, DataRevision, CaseCount int
	StoragePath, SHA256              string
}

func ValidatePublication(input PublishInput, revision, dataRevision int, candidate PublicationCandidate) error {
	if input.Revision != revision || input.ArtifactVersion != candidate.Version || dataRevision != candidate.DataRevision {
		return ErrRevisionConflict
	}
	if candidate.StoragePath == "" || candidate.SHA256 == "" || candidate.CaseCount <= 0 {
		return ErrNotPublished
	}
	return nil
}

// PublicationText makes language fallback and sample rendering explicit. A
// missing non-default translation must never publish another language silently.
func PublicationText(title, markdown, defaultLanguage, language string, statement *Statement, samples []TestOutcome) (string, string, error) {
	if statement != nil {
		markdown = RenderStatement(*statement, SamplesFromOutcomes(samples))
		if statement.Name != "" {
			title = statement.Name
		}
	} else if language != defaultLanguage {
		return "", "", InvalidInput("所选语言尚无已保存题面")
	}
	if strings.TrimSpace(markdown) == "" {
		return "", "", InvalidInput("请先保存完整题面，再发布版本")
	}
	return title, markdown, nil
}
