package httpapi

import (
	"net/http"
	"slices"

	authoringhandler "github.com/RimuruChan/Vertex/server/internal/authoring/handler"
	consolehandler "github.com/RimuruChan/Vertex/server/internal/console/handler"
	contenthandler "github.com/RimuruChan/Vertex/server/internal/content/handler"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	domainhandler "github.com/RimuruChan/Vertex/server/internal/domain/handler"
	identityhandler "github.com/RimuruChan/Vertex/server/internal/identity/handler"
	judgehandler "github.com/RimuruChan/Vertex/server/internal/judge/handler"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	problemsethandler "github.com/RimuruChan/Vertex/server/internal/problemset/handler"
	profilehandler "github.com/RimuruChan/Vertex/server/internal/profile/handler"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/handler"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Dependencies is the HTTP composition boundary. The process entry point owns
// concrete persistence construction; routing only wires injected handlers.
type Dependencies struct {
	Domains        *domainhandler.Handler
	PublicIDs      PublicIDResolver
	ResolveDomain  gin.HandlerFunc
	Auth           *identityhandler.AuthHandler
	Health         *HealthHandler
	Submissions    *submissionhandler.SubmissionHandler
	Problems       *problemhandler.ProblemHandler
	Contests       *contesthandler.ContestHandler
	Editorials     *contenthandler.EditorialHandler
	Discussions    *contenthandler.DiscussionHandler
	AdminProblems  *problemhandler.AdminProblemHandler
	AdminPackages  *authoringhandler.PackageHandler
	ProblemSets    *problemsethandler.SetHandler
	Console        *consolehandler.ConsoleHandler
	Builds         *authoringhandler.BuildHandler
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
	if deps.Domains != nil {
		deps.Domains.RegisterRoutes(api, deps.OptionalAuth, deps.RequireAuth)
	}
	resourceScope := []gin.HandlerFunc{}
	if deps.ResolveDomain != nil {
		resourceScope = append(resourceScope, deps.ResolveDomain)
	}
	resourceScope = append(resourceScope, PublicIDs(deps.PublicIDs))
	deps.Auth.RegisterRoutes(api, deps.RequireAuth)
	if deps.ResolveDomain != nil {
		authoringhandler.RegisterCopyRoutes(api, deps.AdminPackages, deps.RequireAuth, resourceScope...)
	}
	resources := []*gin.RouterGroup{api}
	if deps.ResolveDomain != nil {
		resources = append(resources, api.Group("/domains/:domain"))
	}
	for _, scope := range resources {
		problemhandler.RegisterRoutes(scope, deps.Problems, deps.AdminProblems, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		authoringhandler.RegisterRoutes(scope, deps.AdminPackages, deps.RequireAuth, resourceScope...)
		problemsethandler.RegisterRoutes(scope, deps.ProblemSets, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		deps.Contests.RegisterRoutes(scope, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		deps.Submissions.RegisterRoutes(scope, deps.RequireAuth, resourceScope...)
		contenthandler.RegisterRoutes(scope, deps.Editorials, deps.Discussions, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		deps.Profiles.RegisterRoutes(scope, append([]gin.HandlerFunc{deps.OptionalAuth}, resourceScope...)...)
	}
	consolehandler.RegisterRoutes(api, deps.Console, deps.OptionalAuth, deps.RequireAuth, deps.RequireAdmin, resourceScope...)
	if deps.ResolveDomain != nil {
		consolehandler.RegisterPublicRoutes(api.Group("/domains/:domain"), deps.Console, deps.OptionalAuth, resourceScope...)
	}

	internal := router.Group("/internal")
	deps.Judge.RegisterRoutes(internal, deps.RequireJudge)
	deps.Builds.RegisterInternalRoutes(internal, deps.RequireJudge)
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
