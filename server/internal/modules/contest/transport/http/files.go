package httpapi

import (
	"mime"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// GetProblemFile uses contest access, registration and start-time policy.
//
//	@Summary	Download a file from a pinned contest problem
//	@Tags		contests
//	@Security	BearerAuth
//	@Produce	application/octet-stream
//	@Param		domain			path		string	true	"Domain slug"
//	@Param		id				path		string	true	"Contest number"
//	@Param		problemId		path		string	true	"Contest problem label"
//	@Param		fileId			path		string	true	"Published file ID"
//	@Param		version			query		int		true	"Expected pinned publication"
//	@Success	200				{file}		file
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/domains/{domain}/contests/{id}/problems/{problemId}/files/{fileId} [get]
func (h *ContestHandler) GetProblemFile(c *gin.Context) {
	version, err := strconv.Atoi(c.Query("version"))
	if err != nil || version < 1 {
		httpx.WriteError(c, 400, "request.invalid", "publication version is required")
		return
	}
	item, err := h.service.Problem(c.Request.Context(), httpx.ResourceID(c, "id"), c.Param("problemId"), middleware.CurrentUserID(c), middleware.CurrentRole(c))
	if err != nil {
		h.writeError(c, err, "failed to load contest problem")
		return
	}
	if h.media == nil || version != item.Version {
		httpx.WriteError(c, 404, "contest.not_found", "published file not found")
		return
	}
	file, stream, err := h.media.Open(c.Request.Context(), item.ProblemID, version, c.Param("fileId"))
	if err != nil {
		httpx.WriteError(c, 404, "contest.not_found", "published file not found")
		return
	}
	defer stream.Close()
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Cache-Control", "private, no-store")
	c.DataFromReader(http.StatusOK, file.Size, file.MediaType, stream, nil)
}
