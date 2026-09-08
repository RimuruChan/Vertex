package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http/dto"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

const (
	maxBuildControlBody  = 64 << 10
	maxBuildProgressBody = 1 << 20
	maxBuildResultBody   = 16 << 20
	// Lease identity for the binary artifact upload travels in headers because
	// the request body is the archive itself.
	workerIDHeader   = "X-Vertex-Worker-Id"
	leaseTokenHeader = "X-Vertex-Lease-Token"
)

// BuildLimits is the resource budget the server hands to build workers. It
// lives here so operators tune it in one place instead of per worker node.
type BuildLimits struct {
	GeneratorTimeMs int
	ValidatorTimeMs int
	SolutionTimeMs  int
	CheckerTimeMs   int
	MemoryLimitKB   int
}

// DefaultBuildLimits are deliberately generous: package sources are written by
// trusted authors, and a slow generator should not fail a build the way a slow
// submission fails a judge run.
func DefaultBuildLimits() BuildLimits {
	return BuildLimits{
		GeneratorTimeMs: 30_000,
		ValidatorTimeMs: 30_000,
		SolutionTimeMs:  60_000,
		CheckerTimeMs:   30_000,
		MemoryLimitKB:   1_048_576,
	}
}

// BuildService is the internal worker protocol boundary.
type BuildService interface {
	Claim(ctx context.Context, workerID string, wait time.Duration) (*authoringdomain.Build, *authoringdomain.Package, error)
	Progress(ctx context.Context, progress authoringdomain.Progress) error
	UploadPackage(ctx context.Context, buildID, problemID, workerID, leaseToken string, archive []byte) (*authoringdomain.PackageUpload, error)
	Complete(ctx context.Context, result authoringdomain.BuildResult, checker string) error
}

// BuildHandler serves the lease-fenced build worker protocol.
type BuildHandler struct {
	service BuildService
	limits  BuildLimits
}

func NewBuildHandler(service BuildService, limits BuildLimits) *BuildHandler {
	return &BuildHandler{service: service, limits: limits}
}

// Claim long-polls for one leased package build.
//
//	@Summary	Claim a package build job
//	@Tags		judge-internal
//	@Accept		json
//	@Produce	json
//	@Security	JudgeServiceAuth
//	@Param		request	body		dto.BuildClaimRequest	true	"Worker and long-poll settings"
//	@Success	200		{object}	dto.BuildJobResponse
//	@Success	204
//	@Failure	400,401,413,503	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/builds/claim [post]
func (h *BuildHandler) Claim(c *gin.Context) {
	var request dto.BuildClaimRequest
	if !httpx.BindJSON(c, &request, maxBuildControlBody, "workerId is required") {
		return
	}
	build, pkg, err := h.service.Claim(
		c.Request.Context(), request.WorkerID, time.Duration(request.WaitSeconds)*time.Second)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, authoringdomain.ErrInvalidInput) {
			httpx.WriteError(c, http.StatusBadRequest, "build.invalid_request", err.Error())
			return
		}
		httpx.WriteError(c, http.StatusServiceUnavailable, "build.claim_failed", "unable to claim a build job")
		return
	}
	if build == nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, dto.BuildJobFromDomain(*build, *pkg, dto.BuildLimitsPayload{
		GeneratorTimeMs: h.limits.GeneratorTimeMs,
		ValidatorTimeMs: h.limits.ValidatorTimeMs,
		SolutionTimeMs:  h.limits.SolutionTimeMs,
		CheckerTimeMs:   h.limits.CheckerTimeMs,
		MemoryLimitKB:   h.limits.MemoryLimitKB,
	}))
}

// Progress renews a live lease and publishes stage progress.
//
//	@Summary	Report package build progress
//	@Tags		judge-internal
//	@Accept		json
//	@Security	JudgeServiceAuth
//	@Param		buildId	path	string						true	"Build ID"
//	@Param		request	body	dto.BuildProgressRequest	true	"Lease identity and progress"
//	@Success	204
//	@Failure	400,401,409,413	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/builds/{buildId}/progress [post]
func (h *BuildHandler) Progress(c *gin.Context) {
	var request dto.BuildProgressRequest
	if !httpx.BindJSON(c, &request, maxBuildProgressBody, "workerId and leaseToken are required") {
		return
	}
	if err := h.service.Progress(c.Request.Context(), request.Domain(c.Param("buildId"))); err != nil {
		writeBuildError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// UploadPackage stores the artifact a leased build produced. The archive is
// installed content-addressed but is not published until the build completes
// successfully.
//
//	@Summary	Upload a built testdata package
//	@Tags		judge-internal
//	@Accept		octet-stream
//	@Produce	json
//	@Security	JudgeServiceAuth
//	@Param		buildId					path		string	true	"Build ID"
//	@Param		problemId				query		string	true	"Problem ID"
//	@Param		X-Vertex-Worker-Id		header		string	true	"Worker ID"
//	@Param		X-Vertex-Lease-Token	header		string	true	"Lease token"
//	@Success	200						{object}	dto.BuildPackageResponse
//	@Failure	400,401,409,413			{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/builds/{buildId}/package [post]
func (h *BuildHandler) UploadPackage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, authoringfiles.MaxPackageBytes)
	archive, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.WriteError(c, http.StatusRequestEntityTooLarge, "build.package_too_large",
			"build package exceeds the configured limit")
		return
	}
	upload, err := h.service.UploadPackage(
		c.Request.Context(), c.Param("buildId"), c.Query("problemId"),
		c.GetHeader(workerIDHeader), c.GetHeader(leaseTokenHeader), archive)
	if err != nil {
		writeBuildError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.BuildPackageResponse{
		StoragePath: upload.StoragePath, SHA256: upload.SHA256,
		CaseCount: upload.CaseCount, Checker: upload.Checker,
	})
}

// Complete records a fenced build result and publishes the artifact on success.
//
//	@Summary	Complete a package build
//	@Tags		judge-internal
//	@Accept		json
//	@Security	JudgeServiceAuth
//	@Param		buildId	path	string					true	"Build ID"
//	@Param		request	body	dto.BuildResultRequest	true	"Build result"
//	@Success	204
//	@Failure	400,401,409,413	{object}	httpx.ErrorResponse
//	@Router		/internal/judge/v1/builds/{buildId}/result [put]
func (h *BuildHandler) Complete(c *gin.Context) {
	var request dto.BuildResultRequest
	if !httpx.BindJSON(c, &request, maxBuildResultBody, "invalid build result") {
		return
	}
	if err := h.service.Complete(c.Request.Context(), request.Domain(c.Param("buildId")), request.Checker); err != nil {
		writeBuildError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeBuildError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, authoringdomain.ErrStaleLease):
		httpx.WriteError(c, http.StatusConflict, "build.stale_lease", authoringdomain.ErrStaleLease.Error())
	case errors.Is(err, authoringdomain.ErrPackageTooBig):
		httpx.WriteError(c, http.StatusRequestEntityTooLarge, "build.package_too_large", err.Error())
	case errors.Is(err, authoringdomain.ErrPackageTarget):
		httpx.WriteError(c, http.StatusConflict, "build.problem_mismatch", err.Error())
	case errors.Is(err, authoringdomain.ErrInvalidInput):
		httpx.WriteError(c, http.StatusBadRequest, "build.invalid_request", err.Error())
	default:
		httpx.WriteError(c, http.StatusInternalServerError, "build.persistence_failed", "build result persistence failed")
	}
}
