package handler

import (
	"errors"
	"net/http"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/contest/dto"
	"github.com/RimuruChan/Vertex/server/internal/httpx"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/gin-gonic/gin"
)

type ContestHandler struct{ service *contest.Service }

func NewContestHandler(service *contest.Service) *ContestHandler {
	return &ContestHandler{service: service}
}

// @Summary	List visible contests
// @Tags		contests
// @Produce	json
// @Param		page	query		int	false	"Page"
// @Param		size	query		int	false	"Page size"
// @Success	200		{object}	httpx.ListResponse[dto.ContestResponse]
// @Router		/api/contests [get]
func (h *ContestHandler) List(c *gin.Context) { h.list(c, false) }

// @Summary	List all contests
// @Tags		admin
// @Produce	json
// @Security	BearerAuth
// @Param		page	query		int	false	"Page"
// @Param		size	query		int	false	"Page size"
// @Success	200		{object}	httpx.ListResponse[dto.ContestResponse]
// @Failure	401,403	{object}	httpx.ErrorResponse
// @Router		/api/admin/contests [get]
func (h *ContestHandler) ListAdmin(c *gin.Context) { h.list(c, true) }

func (h *ContestHandler) list(c *gin.Context, admin bool) {
	page, size := pagination(c)
	items, total, err := h.service.List(c.Request.Context(), size, (page-1)*size, admin)
	if err != nil {
		writeAPIError(c, http.StatusInternalServerError, "contest.list_failed", "failed to list contests")
		return
	}
	c.JSON(http.StatusOK, httpx.ListResponse[dto.ContestResponse]{Items: dto.FromContests(items), Total: total})
}

// @Summary	Get contest
// @Tags		contests
// @Produce	json
// @Param		id	path		string	true	"Contest ID"
// @Success	200	{object}	dto.ContestDetailsResponse
// @Failure	404	{object}	httpx.ErrorResponse
// @Router		/api/contests/{id} [get]
func (h *ContestHandler) Get(c *gin.Context) { h.get(c, false) }

// @Summary	Get contest as admin
// @Tags		admin
// @Produce	json
// @Security	BearerAuth
// @Param		id			path		string	true	"Contest ID"
// @Success	200			{object}	dto.ContestDetailsResponse
// @Failure	401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/admin/contests/{id} [get]
func (h *ContestHandler) GetAdmin(c *gin.Context) { h.get(c, true) }

func (h *ContestHandler) get(c *gin.Context, adminView bool) {
	details, err := h.service.Details(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c), adminView,
	)
	if err != nil {
		h.writeError(c, err, "failed to load contest")
		return
	}
	c.JSON(http.StatusOK, dto.ContestDetailsResponse{
		Contest: dto.FromContest(*details.Contest), Problems: dto.FromContestProblems(details.Problems),
	})
}

// @Summary	Create contest
// @Tags		admin
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		request		body		dto.ContestUpsertRequest	true	"Contest"
// @Success	201			{object}	dto.ContestResponse
// @Failure	400,401,403	{object}	httpx.ErrorResponse
// @Router		/api/admin/contests [post]
func (h *ContestHandler) Create(c *gin.Context) {
	var request dto.ContestUpsertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid contest payload")
		return
	}
	item, err := h.service.Create(c.Request.Context(), middleware.CurrentUserID(c), contestInput(request))
	if err != nil {
		h.writeError(c, err, "failed to create contest")
		return
	}
	c.JSON(http.StatusCreated, dto.FromContest(*item))
}

// @Summary	Update contest
// @Tags		admin
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		id				path		string						true	"Contest ID"
// @Param		request			body		dto.ContestUpsertRequest	true	"Contest"
// @Success	200				{object}	dto.ContestResponse
// @Failure	400,401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/admin/contests/{id} [put]
func (h *ContestHandler) Update(c *gin.Context) {
	var request dto.ContestUpsertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid contest payload")
		return
	}
	item, err := h.service.Update(c.Request.Context(), c.Param("id"), contestInput(request))
	if err != nil {
		h.writeError(c, err, "failed to update contest")
		return
	}
	c.JSON(http.StatusOK, dto.FromContest(*item))
}

// @Summary	Replace contest problems
// @Tags		admin
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		id			path		string						true	"Contest ID"
// @Param		request		body		dto.ContestProblemsRequest	true	"Problem IDs"
// @Success	200			{object}	httpx.StatusResponse
// @Failure	400,401,403	{object}	httpx.ErrorResponse
// @Router		/api/admin/contests/{id}/problems [put]
func (h *ContestHandler) SetProblems(c *gin.Context) {
	var request dto.ContestProblemsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "problemIds required")
		return
	}
	if err := h.service.SetProblems(c.Request.Context(), c.Param("id"), request.ProblemIDs); err != nil {
		h.writeError(c, err, "failed to set contest problems")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "ok"})
}

// @Summary	Register for contest
// @Tags		contests
// @Accept		json
// @Produce	json
// @Security	BearerAuth
// @Param		id				path		string							true	"Contest ID"
// @Param		request			body		dto.ContestRegistrationRequest	false	"Contest password"
// @Success	200				{object}	httpx.StatusResponse
// @Failure	400,401,403,404	{object}	httpx.ErrorResponse
// @Router		/api/contests/{id}/register [post]
func (h *ContestHandler) Register(c *gin.Context) {
	var request dto.ContestRegistrationRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			writeAPIError(c, http.StatusBadRequest, "request.invalid", "invalid registration payload")
			return
		}
	}
	err := h.service.Register(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c), request.Password,
	)
	if err != nil {
		h.writeError(c, err, "failed to register for contest")
		return
	}
	c.JSON(http.StatusOK, httpx.StatusResponse{Status: "registered"})
}

// @Summary	Get contest registration
// @Tags		contests
// @Produce	json
// @Security	BearerAuth
// @Param		id		path		string	true	"Contest ID"
// @Success	200		{object}	dto.RegistrationResponse
// @Failure	401,404	{object}	httpx.ErrorResponse
// @Router		/api/contests/{id}/registration [get]
func (h *ContestHandler) Registration(c *gin.Context) {
	registered, err := h.service.Registration(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c),
	)
	if err != nil {
		h.writeError(c, err, "failed to check contest registration")
		return
	}
	c.JSON(http.StatusOK, dto.RegistrationResponse{Registered: registered})
}

// @Summary	Get contest rankboard
// @Tags		contests
// @Produce	json
// @Param		id			path		string	true	"Contest ID"
// @Param		frozen		query		bool	false	"Force frozen view"
// @Success	200			{object}	dto.RankboardResponse
// @Failure	400,403,404	{object}	httpx.ErrorResponse
// @Router		/api/contests/{id}/rankboard [get]
func (h *ContestHandler) Rankboard(c *gin.Context) {
	var frozen *bool
	switch c.Query("frozen") {
	case "":
	case "true":
		value := true
		frozen = &value
	case "false":
		value := false
		frozen = &value
	default:
		writeAPIError(c, http.StatusBadRequest, "request.invalid", "frozen must be true or false")
		return
	}
	board, err := h.service.Rankboard(
		c.Request.Context(), c.Param("id"), middleware.CurrentUserID(c), middleware.CurrentRole(c), frozen,
	)
	if err != nil {
		h.writeError(c, err, "failed to compute rankboard")
		return
	}
	c.JSON(http.StatusOK, dto.FromRankboard(board))
}

func (h *ContestHandler) writeError(c *gin.Context, err error, fallback string) {
	var validation *contest.ValidationError
	switch {
	case errors.As(err, &validation):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", validation.Message)
	case errors.Is(err, contest.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "contest.not_found", "contest not found")
	case errors.Is(err, contest.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "contest.forbidden", "contest is private")
	case errors.Is(err, contest.ErrRegistrationClosed):
		writeAPIError(c, http.StatusBadRequest, "contest.registration_closed", err.Error())
	case errors.Is(err, contest.ErrInvalidPassword):
		writeAPIError(c, http.StatusForbidden, "contest.invalid_password", err.Error())
	case errors.Is(err, contest.ErrRegistrationNeeded):
		writeAPIError(c, http.StatusForbidden, "contest.registration_required", err.Error())
	case errors.Is(err, contest.ErrRankboardHidden):
		writeAPIError(c, http.StatusForbidden, "contest.rankboard_hidden", err.Error())
	default:
		writeAPIError(c, http.StatusInternalServerError, "contest.operation_failed", fallback)
	}
}

func contestInput(request dto.ContestUpsertRequest) contest.UpsertInput {
	return request.UpsertInput()
}

func pagination(c *gin.Context) (int, int) {
	page := parseIntDefault(c.Query("page"), 1)
	size := parseIntDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}
