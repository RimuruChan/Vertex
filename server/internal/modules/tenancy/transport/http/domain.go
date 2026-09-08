package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	dto "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *tenancyapp.Service }

func NewHandler(service *tenancyapp.Service) *Handler { return &Handler{service: service} }
func actor(c *gin.Context) string                     { return middleware.CurrentUserID(c) }
func slug(c *gin.Context) string                      { return c.Param("domain") }
func filters(c *gin.Context) tenancydomain.Filters {
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("size"))
	if page < 1 || page > 100000 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 50
	}
	return tenancydomain.Filters{Keyword: c.Query("keyword"), Limit: size, Offset: (page - 1) * size}
}
func fail(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "internal.error", "服务器内部错误"
	switch {
	case errors.Is(err, tenancydomain.ErrNotFound):
		status, code, message = 404, "domain.not_found", "域或资源不存在"
	case errors.Is(err, tenancydomain.ErrUnauthenticated):
		status, code, message = 401, "auth.required", "请先登录"
	case errors.Is(err, tenancydomain.ErrForbidden):
		status, code, message = 403, "domain.forbidden", "没有执行此操作的域权限"
	case errors.Is(err, tenancydomain.ErrConflict):
		status, code, message = 409, "domain.conflict", "操作与现有成员、角色或所有权冲突"
	case errors.Is(err, tenancydomain.ErrInvalid):
		status, code, message = 400, "domain.invalid", "输入不合法"
		var invalid *tenancydomain.ValidationError
		if errors.As(err, &invalid) {
			message = invalid.Message
		}
	default:
		_ = c.Error(err)
	}
	httpx.WriteError(c, status, code, message)
}
func bind(c *gin.Context, value any) bool {
	return httpx.BindJSON(c, value, 16<<10, "invalid domain request")
}
func status(c *gin.Context, err error) {
	if err != nil {
		fail(c, err)
	} else {
		c.JSON(200, gin.H{"status": "ok"})
	}
}

// List lists visible domains, including the default official domain.
//
//	@Summary	List domains
//	@Tags		domains
//	@Param		page	query		int		false	"Page"
//	@Param		size	query		int		false	"Page size"
//	@Param		keyword	query		string	false	"Name or slug"
//	@Success	200		{object}	httpx.ListResponse[dto.DomainResponse]
//	@Failure	401		{object}	httpx.ErrorResponse
//	@Router		/api/domains [get]
func (h *Handler) List(c *gin.Context) {
	values, total, err := h.service.List(c.Request.Context(), actor(c), filters(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, httpx.ListResponse[dto.DomainResponse]{Items: dto.FromDomains(values), Total: total})
}

// @Summary	Read a domain and effective capabilities
// @Tags		domains
// @Param		domain	path		string	true	"Domain slug"
// @Success	200		{object}	dto.DomainResponse
// @Failure	401,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain} [get]
func (h *Handler) Get(c *gin.Context) {
	value, err := h.service.Get(c.Request.Context(), slug(c), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, dto.FromDomain(value))
}

// @Summary	Create a domain
// @Tags		domains
// @Security	BearerAuth
// @Accept		json
// @Param		request			body		dto.CreateDomainRequest	true	"Domain"
// @Success	201				{object}	dto.DomainResponse
// @Failure	400,401,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains [post]
func (h *Handler) Create(c *gin.Context) {
	var r dto.CreateDomainRequest
	if !bind(c, &r) {
		return
	}
	value, err := h.service.Create(c.Request.Context(), actor(c), tenancydomain.CreateInput{Slug: r.Slug, Name: r.Name, Description: r.Description, Visibility: r.Visibility, JoinPolicy: r.JoinPolicy})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, dto.FromDomain(value))
}

// @Summary	Update domain settings
// @Tags		domains
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string					true	"Domain slug"
// @Param		request					body		dto.UpdateDomainRequest	true	"Settings"
// @Success	200						{object}	dto.DomainResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain} [put]
func (h *Handler) Update(c *gin.Context) {
	var r dto.UpdateDomainRequest
	if !bind(c, &r) {
		return
	}
	value, err := h.service.Update(c.Request.Context(), slug(c), actor(c), tenancydomain.UpdateInput{Name: r.Name, Description: r.Description, Visibility: r.Visibility, JoinPolicy: r.JoinPolicy})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, dto.FromDomain(value))
}

// @Summary	Join, request membership, or accept a domain invitation
// @Tags		domains
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Success	200			{object}	dto.DomainResponse
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/membership [post]
func (h *Handler) Join(c *gin.Context) {
	value, err := h.service.Join(c.Request.Context(), slug(c), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, dto.FromDomain(value))
}

// @Summary	Transfer domain ownership to an active member
// @Tags		domains
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string				true	"Domain slug"
// @Param		request					body		dto.OwnerRequest	true	"New owner"
// @Success	200						{object}	httpx.StatusResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/owner [put]
func (h *Handler) Transfer(c *gin.Context) {
	var r dto.OwnerRequest
	if bind(c, &r) {
		status(c, h.service.Transfer(c.Request.Context(), slug(c), actor(c), r.Username))
	}
}

// @Summary	Archive or restore a non-official domain
// @Tags		domains
// @Security	BearerAuth
// @Accept		json
// @Param		domain				path		string				true	"Domain slug"
// @Param		request				body		dto.ArchiveRequest	true	"Archive state"
// @Success	200					{object}	httpx.StatusResponse
// @Failure	400,401,403,404,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/archive [put]
func (h *Handler) Archive(c *gin.Context) {
	var r dto.ArchiveRequest
	if bind(c, &r) {
		status(c, h.service.Archive(c.Request.Context(), slug(c), actor(c), r.Archived))
	}
}

// @Summary	List domain members
// @Tags		domains
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Param		page		query		int		false	"Page"
// @Param		size		query		int		false	"Page size"
// @Param		keyword		query		string	false	"Username"
// @Success	200			{object}	httpx.ListResponse[dto.MemberResponse]
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/members [get]
func (h *Handler) Members(c *gin.Context) {
	values, total, err := h.service.Members(c.Request.Context(), slug(c), actor(c), filters(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, httpx.ListResponse[dto.MemberResponse]{Items: dto.FromMembers(values), Total: total})
}

// @Summary	Invite, approve, change role, or suspend a domain member
// @Tags		domains
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string				true	"Domain slug"
// @Param		username				path		string				true	"Username"
// @Param		request					body		dto.MemberRequest	true	"Membership"
// @Success	200						{object}	httpx.StatusResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/members/{username} [put]
func (h *Handler) SetMember(c *gin.Context) {
	var r dto.MemberRequest
	if bind(c, &r) {
		status(c, h.service.SetMember(c.Request.Context(), slug(c), actor(c), tenancydomain.MemberInput{Username: c.Param("username"), RoleKey: r.RoleKey, Status: r.Status}))
	}
}

// @Summary	List the domain permission catalogue
// @Tags		domains
// @Success	200	{object}	httpx.ListResponse[dto.PermissionResponse]
// @Router		/api/domain-permissions [get]
func (h *Handler) Permissions(c *gin.Context) {
	items := []dto.PermissionResponse{}
	for _, p := range tenancydomain.PermissionCatalog() {
		items = append(items, dto.PermissionResponse{Key: p.Key, Label: p.Label})
	}
	c.JSON(200, httpx.ListResponse[dto.PermissionResponse]{Items: items, Total: len(items)})
}

// @Summary	List domain roles
// @Tags		domains
// @Security	BearerAuth
// @Param		domain		path		string	true	"Domain slug"
// @Success	200			{object}	httpx.ListResponse[dto.RoleResponse]
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/roles [get]
func (h *Handler) Roles(c *gin.Context) {
	values, err := h.service.Roles(c.Request.Context(), slug(c), actor(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, httpx.ListResponse[dto.RoleResponse]{Items: dto.FromRoles(values), Total: len(values)})
}

// @Summary	Create or replace a custom domain role
// @Tags		domains
// @Security	BearerAuth
// @Accept		json
// @Param		domain					path		string			true	"Domain slug"
// @Param		role					path		string			true	"Role key"
// @Param		request					body		dto.RoleRequest	true	"Role"
// @Success	200						{object}	httpx.StatusResponse
// @Failure	400,401,403,404,409,413	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/roles/{role} [put]
func (h *Handler) SaveRole(c *gin.Context) {
	var r dto.RoleRequest
	if bind(c, &r) {
		status(c, h.service.SaveRole(c.Request.Context(), slug(c), actor(c), tenancydomain.RoleInput{Key: c.Param("role"), Name: r.Name, Permissions: r.Permissions}))
	}
}

// @Summary	Delete an unused custom domain role
// @Tags		domains
// @Security	BearerAuth
// @Param		domain			path		string	true	"Domain slug"
// @Param		role			path		string	true	"Role key"
// @Success	200				{object}	httpx.StatusResponse
// @Failure	401,403,404,409	{object}	httpx.ErrorResponse
// @Router		/api/domains/{domain}/roles/{role} [delete]
func (h *Handler) DeleteRole(c *gin.Context) {
	status(c, h.service.DeleteRole(c.Request.Context(), slug(c), actor(c), c.Param("role")))
}
