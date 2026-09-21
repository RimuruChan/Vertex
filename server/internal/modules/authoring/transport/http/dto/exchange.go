package dto

type PackageExportRequest struct {
	Format   string `json:"format" binding:"required"`
	Revision int64  `json:"revision,omitempty"`
}
