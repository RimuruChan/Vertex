package dto

import (
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
)

type PublishRequest struct {
	Revision        *int   `json:"revision" binding:"required,gte=0"`
	ArtifactVersion int    `json:"artifactVersion" binding:"required,gt=0"`
	Language        string `json:"language,omitempty"`
}

type ReleaseResponse struct {
	Version         int       `json:"version"`
	Revision        int       `json:"revision"`
	ArtifactVersion int       `json:"artifactVersion"`
	Language        string    `json:"language"`
	SHA256          string    `json:"sha256"`
	CaseCount       int       `json:"caseCount"`
	CreatedAt       time.Time `json:"createdAt"`
}

func FromRelease(r authoringdomain.Release) ReleaseResponse {
	return ReleaseResponse{Version: r.Version, Revision: r.Revision, ArtifactVersion: r.ArtifactVersion, Language: r.Language, SHA256: r.SHA256, CaseCount: r.CaseCount, CreatedAt: r.CreatedAt}
}
