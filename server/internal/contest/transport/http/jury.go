package httpapi

import (
	"net/http"

	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/contest/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const maxClarificationBody = 64 << 10

// ---------- staff ----------

// ListStaff returns the contest's jury and observers.
//
//	@Summary	List contest staff
//	@Tags		contests
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Contest ID"
//	@Success	200			{object}	httpx.ListResponse[dto.ContestStaffResponse]
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/staff [get]
//	@Router		/api/domains/{domain}/contests/{id}/staff [get]
func (h *ContestHandler) ListStaff(c *gin.Context) {
	if _, err := h.service.RequireStaff(c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c)); err != nil {
		h.writeError(c, err, "failed to authorize roster access")
		return
	}
	staff, err := h.service.ListStaff(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err, "failed to list contest staff")
		return
	}
	items := dto.FromStaff(staff)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ContestStaffResponse]{Items: items, Total: len(items)})
}

// AddStaff grants a contest role to a user by username.
//
//	@Summary	Add contest staff
//	@Tags		contests
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string					true	"Contest ID"
//	@Param		request				body		dto.ContestStaffRequest	true	"Staff member"
//	@Success	200					{object}	dto.ContestStaffResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/staff [post]
//	@Router		/api/domains/{domain}/contests/{id}/staff [post]
func (h *ContestHandler) AddStaff(c *gin.Context) {
	if !h.requireManageAccess(c) {
		return
	}
	var request dto.ContestStaffRequest
	if !httpx.BindJSON(c, &request, maxContestControlBody, "username and role are required") {
		return
	}
	added, err := h.service.AddStaff(c.Request.Context(), c.Param("id"), request.Username, request.Role)
	if err != nil {
		h.writeError(c, err, "failed to add contest staff")
		return
	}
	c.JSON(http.StatusOK, dto.FromStaff([]contestdomain.Staff{*added})[0])
}

// RemoveStaff revokes a contest role.
//
//	@Summary	Remove contest staff
//	@Tags		contests
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Contest ID"
//	@Param		userId		path		string	true	"User ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Param		domain		path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/staff/{userId} [delete]
//	@Router		/api/domains/{domain}/contests/{id}/staff/{userId} [delete]
func (h *ContestHandler) RemoveStaff(c *gin.Context) {
	if !h.requireManageAccess(c) {
		return
	}
	if err := h.service.RemoveStaff(c.Request.Context(), c.Param("id"), c.Param("userId")); err != nil {
		h.writeError(c, err, "failed to remove contest staff")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "removed"})
}

// ---------- clarifications ----------

// ListClarifications returns the threads the caller may read: everything for
// staff, own threads plus announcements for a contestant.
//
//	@Summary	List contest clarifications
//	@Tags		contests
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"Contest ID"
//	@Success	200		{object}	httpx.ListResponse[dto.ClarificationResponse]
//	@Failure	401,404	{object}	httpx.ErrorResponse
//	@Param		domain	path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/clarifications [get]
//	@Router		/api/domains/{domain}/contests/{id}/clarifications [get]
func (h *ContestHandler) ListClarifications(c *gin.Context) {
	items, err := h.service.Clarifications(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c))
	if err != nil {
		h.writeError(c, err, "failed to list clarifications")
		return
	}
	responses := dto.FromClarifications(items)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ClarificationResponse]{
		Items: responses, Total: len(responses),
	})
}

// Ask records a contestant's question to the jury.
//
//	@Summary	Ask the jury a question
//	@Tags		contests
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string						true	"Contest ID"
//	@Param		request				body		dto.ClarificationAskRequest	true	"Question"
//	@Success	201					{object}	dto.ClarificationResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/clarifications [post]
//	@Router		/api/domains/{domain}/contests/{id}/clarifications [post]
func (h *ContestHandler) Ask(c *gin.Context) {
	var request dto.ClarificationAskRequest
	if !httpx.BindJSON(c, &request, maxClarificationBody, "body is required") {
		return
	}
	created, err := h.service.Ask(c.Request.Context(),
		request.Input(c.Param("id"), middleware.CurrentUserID(c)))
	if err != nil {
		h.writeError(c, err, "failed to submit the question")
		return
	}
	c.JSON(http.StatusCreated, dto.FromClarification(*created))
}

// Reply records a jury answer, or an announcement when no thread is given.
//
//	@Summary	Answer a clarification or announce
//	@Tags		contests
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string							true	"Contest ID"
//	@Param		request				body		dto.ClarificationReplyRequest	true	"Answer"
//	@Success	201					{object}	dto.ClarificationResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Param		domain				path		string	true	"Domain slug"
//	@Router		/api/contests/{id}/clarifications/reply [post]
//	@Router		/api/domains/{domain}/contests/{id}/clarifications/reply [post]
func (h *ContestHandler) Reply(c *gin.Context) {
	if _, err := h.requireJury(c); err != nil {
		return
	}
	var request dto.ClarificationReplyRequest
	if !httpx.BindJSON(c, &request, maxClarificationBody, "body is required") {
		return
	}
	created, err := h.service.Reply(c.Request.Context(),
		request.Input(c.Param("id"), middleware.CurrentUserID(c)))
	if err != nil {
		h.writeError(c, err, "failed to publish the answer")
		return
	}
	c.JSON(http.StatusCreated, dto.FromClarification(*created))
}

// requireJury resolves the caller's contest role and writes the error response
// itself, so each handler above stays a straight-line function.
func (h *ContestHandler) requireJury(c *gin.Context) (contestdomain.Viewer, error) {
	viewer, err := h.service.RequireJury(c.Request.Context(), c.Param("id"),
		middleware.CurrentUserID(c), middleware.CurrentRole(c))
	if err != nil {
		h.writeError(c, err, "failed to authorize the request")
	}
	return viewer, err
}

func (h *ContestHandler) requireManageAccess(c *gin.Context) bool {
	if err := h.service.RequireManageAccess(c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c)); err != nil {
		h.writeError(c, err, "failed to authorize access management")
		return false
	}
	return true
}
