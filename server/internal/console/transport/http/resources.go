package httpapi

import (
	"net/http"
	"strconv"

	consoledomain "github.com/RimuruChan/Vertex/server/internal/console/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/console/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/gin-gonic/gin"
)

// Reject unauthorized resource management before reading request bodies.
func (h *ConsoleHandler) RequireResourceManagement(c *gin.Context) {
	if err := h.service.RequireResourceManagement(c.Request.Context(), c.Request.Method != http.MethodGet); err != nil {
		h.writeError(c, err, "resource authorization failed")
		c.Abort()
		return
	}
	c.Next()
}

// @Summary	Read a domain tag
// @Tags		tags
// @Produce	json
// @Security	BearerAuth
// @Param		domain			path		string	true	"Domain slug"
// @Param		id				path		int		true	"Tag ID"
// @Success	200				{object}	dto.TagCatalogResponse
// @Failure	400,401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/admin/tags/{id} [get]
// @Router		/api/domains/{domain}/admin/tags/{id} [get]
func (h *ConsoleHandler) GetTag(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		writeAPIError(c, 400, "request.invalid", "invalid tag ID")
		return
	}
	item, err := h.service.Tag(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "read tag failed")
		return
	}
	c.JSON(http.StatusOK, dto.FromTag(*item))
}

// @Summary	Create a domain tag
// @Tags		tags
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		domain			path		string					true	"Domain slug"
// @Param		request			body		dto.TagRenameRequest	true	"Tag name"
// @Success	201				{object}	dto.TagCatalogResponse
// @Failure	400,401,403,413	{object}	httpx.ErrorResponse
// @Router		/api/admin/tags [post]
// @Router		/api/domains/{domain}/admin/tags [post]
func (h *ConsoleHandler) CreateTag(c *gin.Context) {
	var request dto.TagRenameRequest
	if !httpx.BindJSON(c, &request, maxConsoleControlBody, "name is required") {
		return
	}
	item, err := h.service.CreateTag(c.Request.Context(), request.Name)
	if err != nil {
		h.writeError(c, err, "create tag failed")
		return
	}
	c.JSON(http.StatusCreated, dto.FromTag(*item))
}

// @Summary	List domain announcements including drafts
// @Tags		announcements
// @Produce	json
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Param		page		query		int		false	"Page"
// @Param		size		query		int		false	"Page size"
// @Param		keyword		query		string	false	"Title or public number"
// @Success	200			{object}	httpx.ListResponse[dto.AnnouncementResponse]
// @Failure	400,401,403	{object}	httpx.ErrorResponse
// @Router		/api/admin/announcements [get]
// @Router		/api/domains/{domain}/admin/announcements [get]
func (h *ConsoleHandler) ListManagedAnnouncements(c *gin.Context) { h.listAnnouncements(c, true) }

func (h *ConsoleHandler) listAnnouncements(c *gin.Context, manage bool) {
	page, size := pagination(c)
	if c.Query("size") == "" && c.Query("limit") != "" {
		if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 && limit <= 100 {
			size = limit
		}
	}
	items, total, err := h.service.AnnouncementPage(c.Request.Context(), manage, consoledomain.AnnouncementFilters{Limit: size, Offset: (page - 1) * size, Keyword: c.Query("keyword")})
	if err != nil {
		h.writeError(c, err, "list announcements failed")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.AnnouncementResponse]{Items: dto.FromAnnouncements(items), Total: total})
}

// @Summary	Read a published domain announcement
// @Tags		announcements
// @Produce	json
// @Param		domain		path		string	true	"Domain slug"
// @Param		id			path		string	true	"Announcement number or ID"
// @Success	200			{object}	dto.AnnouncementResponse
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/announcements/{id} [get]
// @Router		/api/domains/{domain}/announcements/{id} [get]
func (h *ConsoleHandler) GetAnnouncement(c *gin.Context) { h.getAnnouncement(c, false) }

// @Summary	Read a domain announcement for management
// @Tags		announcements
// @Produce	json
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Param		id			path		string	true	"Announcement number or ID"
// @Success	200			{object}	dto.AnnouncementResponse
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/admin/announcements/{id} [get]
// @Router		/api/domains/{domain}/admin/announcements/{id} [get]
func (h *ConsoleHandler) GetManagedAnnouncement(c *gin.Context) { h.getAnnouncement(c, true) }

func (h *ConsoleHandler) getAnnouncement(c *gin.Context, manage bool) {
	item, err := h.service.Announcement(c.Request.Context(), c.Param("id"), manage)
	if err != nil {
		h.writeError(c, err, "read announcement failed")
		return
	}
	c.JSON(http.StatusOK, dto.FromAnnouncement(*item))
}
