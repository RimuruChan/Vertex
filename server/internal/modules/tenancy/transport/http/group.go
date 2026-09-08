package httpapi

import (
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

// @Summary	List groups in a domain
// @Tags		domain-groups
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Param		page		query		int		false	"Page"
// @Param		size		query		int		false	"Page size"
// @Param		keyword		query		string	false	"Name"
// @Success	200			{object}	httpx.ListResponse[dto.GroupResponse]
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups [get]
func (h *Handler) Groups(c *gin.Context) {
	values, total, err := h.service.Groups(c.Request.Context(), slug(c), actor(c), filters(c))
	if err != nil {
		fail(c, err)
		return
	}
	scope, err := h.service.Get(c.Request.Context(), slug(c), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, httpx.ListResponse[dto.GroupResponse]{Items: dto.FromGroups(values, scope), Total: total})
}

// @Summary	Read a group in a domain
// @Tags		domain-groups
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Param		group		path		string	true	"Group number or UUID"
// @Success	200			{object}	dto.GroupResponse
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group} [get]
func (h *Handler) Group(c *gin.Context) {
	value, scope, err := h.service.Group(c.Request.Context(), slug(c), actor(c), c.Param("group"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, dto.FromGroup(value, scope))
}

// @Summary	Create a group in a domain
// @Tags		domain-groups
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string				true	"Domain slug"
// @Param		request					body		dto.GroupRequest	true	"Group"
// @Success	201						{object}	dto.GroupResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups [post]
func (h *Handler) CreateGroup(c *gin.Context) {
	var r dto.GroupRequest
	if !bind(c, &r) {
		return
	}
	value, err := h.service.CreateGroup(c.Request.Context(), slug(c), actor(c), tenancydomain.GroupInput{Name: r.Name, Description: r.Description, OwnerUsername: r.OwnerUsername})
	if err != nil {
		fail(c, err)
		return
	}
	scope, err := h.service.Get(c.Request.Context(), slug(c), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, dto.FromGroup(value, scope))
}

// @Summary	Update group details
// @Tags		domain-groups
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string				true	"Domain slug"
// @Param		group					path		string				true	"Group number or UUID"
// @Param		request					body		dto.GroupRequest	true	"Group details"
// @Success	200						{object}	httpx.StatusResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group} [put]
func (h *Handler) UpdateGroup(c *gin.Context) {
	var r dto.GroupRequest
	if bind(c, &r) {
		status(c, h.service.UpdateGroup(c.Request.Context(), slug(c), actor(c), c.Param("group"), tenancydomain.GroupInput{Name: r.Name, Description: r.Description}))
	}
}

// @Summary	Delete a group without deleting its resources
// @Tags		domain-groups
// @Security	BearerAuth
// @Param		domain			path		string	true	"Domain slug"
// @Param		group			path		string	true	"Group number or UUID"
// @Success	200				{object}	httpx.StatusResponse
// @Failure	401,403,404,409	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group} [delete]
func (h *Handler) DeleteGroup(c *gin.Context) {
	status(c, h.service.DeleteGroup(c.Request.Context(), slug(c), actor(c), c.Param("group")))
}

// @Summary	Transfer group ownership to an active domain member
// @Tags		domain-groups
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string				true	"Domain slug"
// @Param		group					path		string				true	"Group number or UUID"
// @Param		request					body		dto.OwnerRequest	true	"New owner"
// @Success	200						{object}	httpx.StatusResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group}/owner [put]
func (h *Handler) TransferGroup(c *gin.Context) {
	var r dto.OwnerRequest
	if bind(c, &r) {
		status(c, h.service.TransferGroup(c.Request.Context(), slug(c), actor(c), c.Param("group"), r.Username))
	}
}

// @Summary	List active group members
// @Tags		domain-groups
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Param		group		path		string	true	"Group number or UUID"
// @Param		page		query		int		false	"Page"
// @Param		size		query		int		false	"Page size"
// @Param		keyword		query		string	false	"Username"
// @Success	200			{object}	httpx.ListResponse[dto.GroupMemberResponse]
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group}/members [get]
func (h *Handler) GroupMembers(c *gin.Context) {
	values, total, err := h.service.GroupMembers(c.Request.Context(), slug(c), actor(c), c.Param("group"), filters(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, httpx.ListResponse[dto.GroupMemberResponse]{Items: dto.FromGroupMembers(values), Total: total})
}

// @Summary	Add or change a group member
// @Tags		domain-groups
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string					true	"Domain slug"
// @Param		group					path		string					true	"Group number or UUID"
// @Param		username				path		string					true	"Username"
// @Param		request					body		dto.GroupMemberRequest	true	"Membership"
// @Success	200						{object}	httpx.StatusResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group}/members/{username} [put]
func (h *Handler) SetGroupMember(c *gin.Context) {
	var r dto.GroupMemberRequest
	if bind(c, &r) {
		status(c, h.service.SetGroupMember(c.Request.Context(), slug(c), actor(c), c.Param("group"), c.Param("username"), r.Role, false))
	}
}

// @Summary	Remove a group member
// @Tags		domain-groups
// @Security	BearerAuth
// @Param		domain			path		string	true	"Domain slug"
// @Param		group			path		string	true	"Group number or UUID"
// @Param		username		path		string	true	"Username"
// @Success	200				{object}	httpx.StatusResponse
// @Failure	401,403,404,409	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/groups/{group}/members/{username} [delete]
func (h *Handler) RemoveGroupMember(c *gin.Context) {
	status(c, h.service.SetGroupMember(c.Request.Context(), slug(c), actor(c), c.Param("group"), c.Param("username"), "", true))
}
