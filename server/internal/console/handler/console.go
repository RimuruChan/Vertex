// Package handler exposes the site administration console over HTTP.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/RimuruChan/Vertex/server/internal/console"
	"github.com/RimuruChan/Vertex/server/internal/console/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

type ConsoleHandler struct{ service *console.Service }

const (
	maxConsoleControlBody = 64 << 10
	maxAnnouncementBody   = 1 << 20
)

func NewConsoleHandler(service *console.Service) *ConsoleHandler {
	return &ConsoleHandler{service: service}
}

// Stats returns the administration dashboard.
//
//	@Summary	Site statistics and judge queue health
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200		{object}	dto.StatsResponse
//	@Failure	401,403	{object}	httpx.ErrorResponse
//	@Router		/api/admin/stats [get]
func (h *ConsoleHandler) Stats(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "failed to load site statistics")
		return
	}
	c.JSON(http.StatusOK, dto.FromStats(*stats))
}

// ---------- accounts ----------

// ListAccounts searches registered users.
//
//	@Summary	List user accounts
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		keyword		query		string	false	"Username or email"
//	@Param		role		query		string	false	"Role"	Enums(user, admin)
//	@Param		disabled	query		bool	false	"Only blocked accounts"
//	@Param		page		query		int		false	"Page"
//	@Param		size		query		int		false	"Page size"
//	@Success	200			{object}	httpx.ListResponse[dto.AccountResponse]
//	@Failure	400,401,403	{object}	httpx.ErrorResponse
//	@Router		/api/admin/users [get]
func (h *ConsoleHandler) ListAccounts(c *gin.Context) {
	page, size := pagination(c)
	items, total, err := h.service.ListAccounts(c.Request.Context(), console.AccountFilters{
		Keyword: c.Query("keyword"), Role: c.Query("role"),
		OnlyDisabled: c.Query("disabled") == "true",
		Limit:        size, Offset: (page - 1) * size,
	})
	if err != nil {
		h.writeError(c, err, "failed to list accounts")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.AccountResponse]{
		Items: dto.FromAccounts(items), Total: total,
	})
}

// UpdateAccount applies role, rating and block changes.
//
//	@Summary	Update a user account
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string						true	"User ID"
//	@Param		request				body		dto.AccountUpdateRequest	true	"Changes"
//	@Success	200					{object}	dto.AccountResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/users/{id} [patch]
func (h *ConsoleHandler) UpdateAccount(c *gin.Context) {
	var request dto.AccountUpdateRequest
	if !httpx.BindJSON(c, &request, maxConsoleControlBody, "invalid account payload") {
		return
	}
	updated, err := h.service.UpdateAccount(c.Request.Context(),
		middleware.CurrentUserID(c), c.Param("id"), request.Update())
	if err != nil {
		h.writeError(c, err, "failed to update the account")
		return
	}
	c.JSON(http.StatusOK, dto.FromAccount(*updated))
}

// ---------- tags ----------

// ListTags returns the tag catalogue with usage counts.
//
//	@Summary	List tags with usage counts
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200		{object}	httpx.ListResponse[dto.TagCatalogResponse]
//	@Failure	401,403	{object}	httpx.ErrorResponse
//	@Router		/api/admin/tags [get]
func (h *ConsoleHandler) ListTags(c *gin.Context) {
	items, err := h.service.ListTags(c.Request.Context())
	if err != nil {
		h.writeError(c, err, "failed to list tags")
		return
	}
	responses := dto.FromTags(items)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.TagCatalogResponse]{Items: responses, Total: len(responses)})
}

// RenameTag renames a tag, merging it when the target name already exists.
//
//	@Summary	Rename a tag
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		int						true	"Tag ID"
//	@Param		request				body		dto.TagRenameRequest	true	"New name"
//	@Success	200					{object}	dto.TagCatalogResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/tags/{id} [put]
func (h *ConsoleHandler) RenameTag(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid tag ID")
		return
	}
	var request dto.TagRenameRequest
	if !httpx.BindJSON(c, &request, maxConsoleControlBody, "name is required") {
		return
	}
	updated, err := h.service.RenameTag(c.Request.Context(), id, request.Name)
	if err != nil {
		h.writeError(c, err, "failed to rename the tag")
		return
	}
	c.JSON(http.StatusOK, dto.FromTag(*updated))
}

// MergeTag folds one tag into another.
//
//	@Summary	Merge one tag into another
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		int					true	"Source tag ID"
//	@Param		request				body		dto.TagMergeRequest	true	"Target tag"
//	@Success	200					{object}	dto.TagCatalogResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/tags/{id}/merge [post]
func (h *ConsoleHandler) MergeTag(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid tag ID")
		return
	}
	var request dto.TagMergeRequest
	if !httpx.BindJSON(c, &request, maxConsoleControlBody, "targetId is required") {
		return
	}
	merged, err := h.service.MergeTags(c.Request.Context(), id, request.TargetID)
	if err != nil {
		h.writeError(c, err, "failed to merge the tags")
		return
	}
	c.JSON(http.StatusOK, dto.FromTag(*merged))
}

// DeleteTag removes a tag and its problem links.
//
//	@Summary	Delete a tag
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id				path		int	true	"Tag ID"
//	@Success	200				{object}	httpx.StatusResponse
//	@Failure	400,401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/tags/{id} [delete]
func (h *ConsoleHandler) DeleteTag(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid tag ID")
		return
	}
	if err := h.service.DeleteTag(c.Request.Context(), id); err != nil {
		h.writeError(c, err, "failed to delete the tag")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

// ---------- announcements ----------

// ListAnnouncements returns published site notices for everyone; the admin
// route additionally returns drafts.
//
//	@Summary	List site announcements
//	@Tags		announcements
//	@Produce	json
//	@Param		limit	query		int	false	"Maximum notices"
//	@Success	200		{object}	httpx.ListResponse[dto.AnnouncementResponse]
//	@Router		/api/announcements [get]
func (h *ConsoleHandler) ListAnnouncements(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.service.ListAnnouncements(c.Request.Context(),
		middleware.CurrentRole(c) == "admin", limit)
	if err != nil {
		h.writeError(c, err, "failed to list announcements")
		return
	}
	responses := dto.FromAnnouncements(items)
	c.JSON(http.StatusOK, httpx.ListResponse[dto.AnnouncementResponse]{
		Items: responses, Total: len(responses),
	})
}

// CreateAnnouncement publishes a new site notice.
//
//	@Summary	Create a site announcement
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		request			body		dto.AnnouncementUpsertRequest	true	"Announcement"
//	@Success	201				{object}	dto.AnnouncementResponse
//	@Failure	400,401,403,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/announcements [post]
func (h *ConsoleHandler) CreateAnnouncement(c *gin.Context) {
	var request dto.AnnouncementUpsertRequest
	if !httpx.BindJSON(c, &request, maxAnnouncementBody, "title is required") {
		return
	}
	created, err := h.service.CreateAnnouncement(c.Request.Context(),
		middleware.CurrentUserID(c), request.Input())
	if err != nil {
		h.writeError(c, err, "failed to create the announcement")
		return
	}
	c.JSON(http.StatusCreated, dto.FromAnnouncement(*created))
}

// UpdateAnnouncement rewrites a site notice.
//
//	@Summary	Update a site announcement
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id					path		string							true	"Announcement ID"
//	@Param		request				body		dto.AnnouncementUpsertRequest	true	"Announcement"
//	@Success	200					{object}	dto.AnnouncementResponse
//	@Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
//	@Router		/api/admin/announcements/{id} [put]
func (h *ConsoleHandler) UpdateAnnouncement(c *gin.Context) {
	var request dto.AnnouncementUpsertRequest
	if !httpx.BindJSON(c, &request, maxAnnouncementBody, "title is required") {
		return
	}
	updated, err := h.service.UpdateAnnouncement(c.Request.Context(), c.Param("id"), request.Input())
	if err != nil {
		h.writeError(c, err, "failed to update the announcement")
		return
	}
	c.JSON(http.StatusOK, dto.FromAnnouncement(*updated))
}

// DeleteAnnouncement removes a site notice.
//
//	@Summary	Delete a site announcement
//	@Tags		admin
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id			path		string	true	"Announcement ID"
//	@Success	200			{object}	httpx.StatusResponse
//	@Failure	401,403,404	{object}	httpx.ErrorResponse
//	@Router		/api/admin/announcements/{id} [delete]
func (h *ConsoleHandler) DeleteAnnouncement(c *gin.Context) {
	if err := h.service.DeleteAnnouncement(c.Request.Context(), c.Param("id")); err != nil {
		h.writeError(c, err, "failed to delete the announcement")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "deleted"})
}

func (h *ConsoleHandler) writeError(c *gin.Context, err error, fallback string) {
	var validation *console.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, console.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "console.not_found", "resource not found")
	case errors.Is(err, console.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "console.forbidden",
			"an administrator cannot remove their own access")
	default:
		writeAPIError(c, http.StatusInternalServerError, "console.failed", fallback)
	}
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}

func pagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
