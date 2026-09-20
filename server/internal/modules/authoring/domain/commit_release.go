package domain

import "time"

type CommitPublication struct {
	Revision        int64  `json:"revision"`
	CheckID         string `json:"checkId"`
	ExpectedVersion int    `json:"expectedVersion"`
	Language        string `json:"language,omitempty"`
}
type CommitRelease struct {
	Version      int       `json:"version"`
	Revision     int64     `json:"revision"`
	TreeHash     string    `json:"treeHash"`
	CheckID      string    `json:"checkId"`
	ToolchainKey string    `json:"toolchainKey"`
	Language     string    `json:"language"`
	CreatedAt    time.Time `json:"createdAt"`
}
