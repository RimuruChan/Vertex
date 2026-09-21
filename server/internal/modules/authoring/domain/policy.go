package domain

type VisibilityChange struct {
	Visibility         string `json:"visibility"`
	ExpectedVisibility string `json:"expectedVisibility"`
}
type VisibilityState struct {
	Visibility string `json:"visibility"`
}
