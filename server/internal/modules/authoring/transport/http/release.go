package handler

import (
	"net/http"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// Publish promotes only the revision and candidate the owner explicitly reviewed.
//
//	@Summary	Publish a reviewed problem version
//	@Tags		authoring
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id						path		string				true	"Problem ID"
//	@Param		request					body		dto.PublishRequest	true	"Reviewed working and candidate versions"
//	@Success	200						{object}	dto.ReleaseResponse
//	@Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
//	@Param		domain					path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/publish [post]
//	@Router		/api/domains/{domain}/admin/problems/{id}/publish [post]
func (h *PackageHandler) Publish(c *gin.Context) {
	var request dto.PublishRequest
	if !httpx.BindJSON(c, &request, 16<<10, "revision and candidate version are required") {
		return
	}
	release, err := h.service.Publish(c.Request.Context(), c.Param("id"), authoringdomain.PublishInput{Revision: *request.Revision, ArtifactVersion: request.ArtifactVersion, Language: request.Language})
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRelease(*release))
}

// Releases lists immutable publication metadata for package collaborators.
//
//	@Summary	List published problem versions
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Problem ID"
//	@Success	200			{object}	httpx.ListResponse[dto.ReleaseResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/admin/problems/{id}/releases [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/releases [get]
func (h *PackageHandler) Releases(c *gin.Context) {
	versions, err := h.service.Releases(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	items := make([]dto.ReleaseResponse, 0, len(versions))
	for _, version := range versions {
		items = append(items, dto.FromRelease(version))
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ReleaseResponse]{Items: items, Total: len(items)})
}
