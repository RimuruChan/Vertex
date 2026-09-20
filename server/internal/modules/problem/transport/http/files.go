package httpapi

import (
	"mime"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// GetFile serves an approved file of the current visible practice release.
//
//	@Summary	Download a published practice problem file
//	@Tags		problems
//	@Produce	application/octet-stream
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Problem number"
//	@Param		fileId			path		string	true	"Published file ID"
//	@Param		version			query		int		true	"Expected current publication"
//	@Success	200				{file}		file
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/problems/{id}/files/{fileId} [get]
func (h *ProblemHandler) GetFile(c *gin.Context) {
	version, err := strconv.Atoi(c.Query("version"))
	if err != nil || version < 1 {
		httpx.WriteError(c, 400, "request.invalid", "publication version is required")
		return
	}
	item, err := h.service.Get(c.Request.Context(), httpx.ResourceID(c, "id"), middleware.CurrentUserID(c))
	if err != nil {
		writeProblemError(c, err)
		return
	}
	if h.media == nil || version != item.PublishedVersion {
		httpx.WriteError(c, 404, "problem.not_found", "published file not found")
		return
	}
	file, stream, err := h.media.Open(c.Request.Context(), item.ID, version, c.Param("fileId"))
	if err != nil {
		writeProblemError(c, err)
		return
	}
	defer stream.Close()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Cache-Control", "private, no-store")
	c.DataFromReader(http.StatusOK, file.Size, file.MediaType, stream, nil)
}
