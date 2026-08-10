package httpapi

import (
	"net/http"
	"slices"

	contenthandler "github.com/RimuruChan/Vertex/server/internal/content/handler"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	identityhandler "github.com/RimuruChan/Vertex/server/internal/identity/handler"
	judgehandler "github.com/RimuruChan/Vertex/server/internal/judge/handler"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	profilehandler "github.com/RimuruChan/Vertex/server/internal/profile/handler"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/handler"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Dependencies is the HTTP composition boundary. The process entry point owns
// concrete persistence construction; routing only wires injected handlers.
type Dependencies struct {
	Auth           *identityhandler.AuthHandler
	Health         *HealthHandler
	Submissions    *submissionhandler.SubmissionHandler
	Problems       *problemhandler.ProblemHandler
	Contests       *contesthandler.ContestHandler
	Editorials     *contenthandler.EditorialHandler
	Discussions    *contenthandler.DiscussionHandler
	AdminProblems  *problemhandler.AdminProblemHandler
	Profiles       *profilehandler.ProfileHandler
	Judge          *judgehandler.JudgeHandler
	RequireAuth    gin.HandlerFunc
	OptionalAuth   gin.HandlerFunc
	RequireAdmin   gin.HandlerFunc
	RequireJudge   gin.HandlerFunc
	AllowedOrigins []string
	SwaggerEnabled bool
}

// Router wires shared HTTP behavior and delegates domain routes to each
// domain's handler package.
func Router(deps Dependencies) *gin.Engine {
	router := gin.Default()
	router.Use(corsMiddleware(deps.AllowedOrigins))

	if deps.SwaggerEnabled {
		router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}
	router.GET("/api/health", deps.Health.Ready)
	router.GET("/api/health/live", deps.Health.Live)
	router.GET("/api/health/ready", deps.Health.Ready)

	api := router.Group("/api")
	deps.Auth.RegisterRoutes(api, deps.RequireAuth)
	problemhandler.RegisterRoutes(api, deps.Problems, deps.AdminProblems, deps.OptionalAuth, deps.RequireAuth, deps.RequireAdmin)
	deps.Contests.RegisterRoutes(api, deps.OptionalAuth, deps.RequireAuth, deps.RequireAdmin)
	deps.Submissions.RegisterRoutes(api, deps.RequireAuth, deps.RequireAdmin)
	contenthandler.RegisterRoutes(api, deps.Editorials, deps.Discussions, deps.RequireAuth)
	deps.Profiles.RegisterRoutes(api)

	internal := router.Group("/internal")
	deps.Judge.RegisterRoutes(internal, deps.RequireJudge)
	return router
}

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && slices.Contains(allowedOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
