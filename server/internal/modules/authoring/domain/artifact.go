package domain

// CheckArtifact is the internal build output format. Exchange formats such as
// Kattis and Polygon are handled separately, never guessed from this envelope.
type CheckArtifact struct {
	Statements    []ArtifactStatement `json:"statements,omitempty"`
	Dependencies  []SnapshotFile      `json:"dependencies,omitempty"`
	SchemaVersion int                 `json:"schemaVersion"`
	ToolchainKey  string              `json:"toolchainKey"`
	Snapshot      CheckSnapshot       `json:"snapshot"`
	Tests         []ArtifactTest      `json:"tests"`
}
type ArtifactStatement struct {
	ID  string  `json:"id"`
	PDF BlobRef `json:"pdf"`
}
type ArtifactTest struct {
	ID     string  `json:"id"`
	Input  BlobRef `json:"input"`
	Answer BlobRef `json:"answer"`
}
