package dto

import (
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

type CopyRequest struct {
	SourceDomain  string `json:"sourceDomain" binding:"required"`
	SourceProblem string `json:"sourceProblem" binding:"required,numeric"`
	SourceVersion int    `json:"sourceVersion" binding:"required,gt=0"`
	Attribution   string `json:"attribution" binding:"required"`
}

type CopyOriginResponse struct {
	SourceDomainID   string `json:"sourceDomainId"`
	SourceDomainSlug string `json:"sourceDomainSlug"`

	SourceProblemNumber string    `json:"sourceProblemNumber"`
	SourceVersion       int       `json:"sourceVersion"`
	SourceTitle         string    `json:"sourceTitle"`
	SourceSHA256        string    `json:"sourceSha256"`
	Attribution         string    `json:"attribution"`
	CopiedBy            *string   `json:"copiedBy,omitempty"`
	CopiedAt            time.Time `json:"copiedAt"`
}

type ProblemOriginResponse struct {
	Origin *CopyOriginResponse `json:"origin,omitempty"`
}

type CopyResponse struct {
	ProblemID string `json:"problemId"`

	DomainID   string             `json:"domainId"`
	DomainSlug string             `json:"domainSlug"`
	Origin     CopyOriginResponse `json:"origin"`
}

func FromOrigin(origin authoringdomain.CopyOrigin) CopyOriginResponse {
	return CopyOriginResponse{SourceDomainID: origin.SourceDomainID, SourceDomainSlug: origin.SourceDomainSlug,
		SourceProblemNumber: origin.SourceProblemNumber, SourceVersion: origin.SourceVersion,
		SourceTitle: origin.SourceTitle, SourceSHA256: origin.SourceSHA256, Attribution: origin.Attribution, CopiedBy: origin.CopiedBy, CopiedAt: origin.CopiedAt}
}

func FromCopy(result authoringdomain.CopyResult) CopyResponse {
	return CopyResponse{ProblemID: result.PublicID, DomainID: result.DomainID, DomainSlug: result.DomainSlug, Origin: FromOrigin(result.Origin)}
}
