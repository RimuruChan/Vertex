package dto

import "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"

type PublishedFileResponse struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	MediaType   string `json:"mediaType"`
	Purpose     string `json:"purpose"`
	Size        int64  `json:"size"`
	SampleIndex int    `json:"sampleIndex,omitempty"`
	Preview     string `json:"preview,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Binary      bool   `json:"binary,omitempty"`
	Embedded    bool   `json:"embedded,omitempty"`
}

func FromPublishedFiles(files []domain.PublishedFile) []PublishedFileResponse {
	result := make([]PublishedFileResponse, 0, len(files))
	for _, file := range files {
		result = append(result, PublishedFileResponse{ID: file.ID, Path: file.Path, Name: file.Name, MediaType: file.MediaType, Purpose: file.Purpose, Size: file.Size, SampleIndex: file.SampleIndex, Preview: file.Preview, Truncated: file.Truncated, Binary: file.Binary, Embedded: file.Embedded})
	}
	return result
}
