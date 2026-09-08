package domain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	publiciddomain "github.com/RimuruChan/Vertex/server/internal/publicid/domain"
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

func NormalizeCopy(input CopyInput) (CopyInput, error) {
	input.SourceDomain = strings.TrimSpace(input.SourceDomain)
	input.SourceProblem = strings.ToLower(strings.TrimSpace(input.SourceProblem))
	input.Attribution = strings.TrimSpace(input.Attribution)
	if !copyDomainPattern.MatchString(input.SourceDomain) || input.SourceProblem == "" || len(input.SourceProblem) > 64 || input.SourceVersion <= 0 {
		return CopyInput{}, InvalidInput("source domain, problem and published version are required")
	}
	if input.Attribution == "" || len(input.Attribution) > 4096 || !utf8.ValidString(input.Attribution) {
		return CopyInput{}, InvalidInput("a copy attribution of at most 4096 bytes is required")
	}
	if publiciddomain.IsNumber(input.SourceProblem) {
		number, err := strconv.ParseInt(input.SourceProblem, 10, 64)
		if err != nil || number <= 0 {
			return CopyInput{}, InvalidInput("invalid source problem number")
		}
		input.SourceProblem = strconv.FormatInt(number, 10)
	} else if !copyProblemUUID.MatchString(input.SourceProblem) {
		return CopyInput{}, InvalidInput("source problem must be a public number or UUID")
	}
	return input, nil
}
