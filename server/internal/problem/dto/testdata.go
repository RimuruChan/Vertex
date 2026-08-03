package dto

type TestdataUploadResponse struct {
	CaseCount int    `json:"caseCount"`
	SHA256    string `json:"sha256"`
	Checker   string `json:"checker"`
}
