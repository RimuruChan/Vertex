// Package handler exposes private authoring workbenches and the fenced Worker protocol.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/httpx"
	"github.com/gin-gonic/gin"
)

const (
	maxPackageBody        = 8 << 20
	maxPackageControlBody = 64 << 10
)

func pathInt64(c *gin.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	httpx.WriteError(c, status, code, message)
}

// writeAuthoringError maps domain errors onto one HTTP vocabulary so every
// handler above stays a straight-line function.
func writeAuthoringError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenancydomain.ErrForbidden):
		writeAPIError(c, http.StatusForbidden, "authoring.forbidden", "insufficient problem permissions")
	case errors.Is(err, tenancydomain.ErrUnauthenticated):
		writeAPIError(c, http.StatusUnauthorized, "auth.invalid_token", "authentication required")
	case errors.Is(err, authoringdomain.ErrNotFound):
		writeAPIError(c, http.StatusNotFound, "authoring.not_found", "resource not found")
	case errors.Is(err, authoringdomain.ErrInvalidInput):
		writeAPIError(c, http.StatusBadRequest, "request.invalid", err.Error())
	case errors.Is(err, authoringdomain.ErrNotBuildable):
		writeAPIError(c, http.StatusBadRequest, "authoring.not_buildable", err.Error())
	case errors.Is(err, authoringdomain.ErrBuildRunning):
		writeAPIError(c, http.StatusConflict, "authoring.build_running", err.Error())
	case errors.Is(err, authoringdomain.ErrWorkingCopyConflict):
		writeAPIError(c, http.StatusConflict, "authoring.working_copy_conflict", err.Error())
	case errors.Is(err, authoringdomain.ErrMergeRequired):
		writeAPIError(c, http.StatusConflict, "authoring.merge_required", err.Error())
	case errors.Is(err, authoringdomain.ErrMergeOutdated):
		writeAPIError(c, http.StatusConflict, "authoring.merge_outdated", err.Error())
	case errors.Is(err, authoringdomain.ErrRevisionConflict), errors.Is(err, authoringdomain.ErrNotPublished):
		writeAPIError(c, http.StatusConflict, "authoring.publish_conflict", err.Error())
	case errors.Is(err, authoringdomain.ErrPackageTooBig):
		writeAPIError(c, http.StatusRequestEntityTooLarge, "authoring.package_too_large", err.Error())
	case errors.Is(err, authoringdomain.ErrStaleLease):
		writeAPIError(c, http.StatusConflict, "authoring.stale_lease", err.Error())
	default:
		writeAPIError(c, http.StatusInternalServerError, "authoring.failed", "authoring request failed")
	}
}
