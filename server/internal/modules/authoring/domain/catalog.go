package domain

type MaterialPage struct {
	ETag     string         `json:"etag,omitempty"`
	Revision int64          `json:"revision,omitempty"`
	Items    []MaterialView `json:"items"`
	Total    int            `json:"total"`
	Next     string         `json:"next,omitempty"`
}
type MaterialQuery struct {
	Revision int64
	ETag     string
	Kind     string
	After    string
	Limit    int
}
type TestPatch struct {
	TimeLimitMs   *int     `json:"timeLimitMs,omitempty"`
	MemoryLimitKB *int     `json:"memoryLimitKb,omitempty"`
	IsSample      *bool    `json:"isSample,omitempty"`
	IsPretest     *bool    `json:"isPretest,omitempty"`
	Group         *string  `json:"group,omitempty"`
	Points        *float64 `json:"points,omitempty"`
}
type MaterialBatch struct {
	ETag      string     `json:"etag"`
	DeleteIDs []string   `json:"deleteIds,omitempty"`
	TestIDs   []string   `json:"testIds,omitempty"`
	Patch     *TestPatch `json:"patch,omitempty"`
	TestOrder *[]string  `json:"testOrder,omitempty"`
}
