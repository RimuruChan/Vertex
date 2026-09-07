package authoring

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/publicid"
)

type CopyInput struct {
	SourceDomain, SourceProblem, Attribution string
	SourceVersion                            int
}

// CopyOrigin records facts at copy time, not a live authorization relationship.
// It is returned only to collaborators of the destination package.
type CopyOrigin struct {
	SourceDomainID, SourceDomainSlug       string
	SourceProblemID, SourceProblemNumber   string
	SourceTitle, SourceSHA256, Attribution string
	SourceVersion                          int
	CopiedBy                               *string
	CopiedAt                               time.Time
}

type CopyResult struct {
	ProblemID, PublicID, DomainID, DomainSlug string
	Origin                                    CopyOrigin
}

var (
	copyDomainPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
	copyProblemUUID   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

func (s *Service) Copy(ctx context.Context, input CopyInput) (*CopyResult, error) {
	input.SourceDomain = strings.TrimSpace(input.SourceDomain)
	input.SourceProblem = strings.ToLower(strings.TrimSpace(input.SourceProblem))
	input.Attribution = strings.TrimSpace(input.Attribution)
	if !copyDomainPattern.MatchString(input.SourceDomain) || input.SourceProblem == "" || len(input.SourceProblem) > 64 || input.SourceVersion <= 0 {
		return nil, invalid("source domain, problem and published version are required")
	}
	if input.Attribution == "" || len(input.Attribution) > 4096 || !utf8.ValidString(input.Attribution) {
		return nil, invalid("a copy attribution of at most 4096 bytes is required")
	}
	if publicid.IsNumber(input.SourceProblem) {
		number, err := strconv.ParseInt(input.SourceProblem, 10, 64)
		if err != nil || number <= 0 {
			return nil, invalid("invalid source problem number")
		}
		input.SourceProblem = strconv.FormatInt(number, 10)
	} else if !copyProblemUUID.MatchString(input.SourceProblem) {
		return nil, invalid("source problem must be a public number or UUID")
	}
	return s.packages.Copy(ctx, input, s.publisher)
}

func (s *Service) Origin(ctx context.Context, problemID string) (*CopyOrigin, error) {
	return s.packages.Origin(ctx, problemID)
}
