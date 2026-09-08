package handler

import (
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/authoring/transport/http/dto"
	"github.com/gin-gonic/gin"
)

// GetTest reads the complete editable input, not the list's bounded preview.
//
//	@Summary	Read one full test definition
//	@Tags		authoring
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		string	true	"Problem ID"
//	@Param		testId			path		int		true	"Test ID"
//	@Param		domain			path		string	true	"Domain slug"
//	@Success	200				{object}	dto.TestResponse
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/problems/{id}/tests/{testId} [get]
//	@Router		/api/domains/{domain}/admin/problems/{id}/tests/{testId} [get]
func (h *PackageHandler) GetTest(c *gin.Context) {
	id, err := pathInt64(c, "testId")
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid test ID")
		return
	}
	test, err := h.service.Test(c.Request.Context(), c.Param("id"), id)
	if err != nil {
		writeAuthoringError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.FromTest(*test))
}
