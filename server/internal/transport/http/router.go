package httpapi

import (
	"net/http"
	"slices"

	authoringhandler "github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http"
	consolehttp "github.com/RimuruChan/Vertex/server/internal/modules/console/transport/http"
	contenthttp "github.com/RimuruChan/Vertex/server/internal/modules/content/transport/http"
	contesthttp "github.com/RimuruChan/Vertex/server/internal/modules/contest/transport/http"
	identityhttp "github.com/RimuruChan/Vertex/server/internal/modules/identity/transport/http"
	judgehttp "github.com/RimuruChan/Vertex/server/internal/modules/judge/transport/http"
	problemhttp "github.com/RimuruChan/Vertex/server/internal/modules/problem/transport/http"
	sethttp "github.com/RimuruChan/Vertex/server/internal/modules/problemset/transport/http"
	profilehttp "github.com/RimuruChan/Vertex/server/internal/modules/profile/transport/http"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/modules/submission/transport/http"
	tenancyhttp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/transport/http"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Dependencies is the HTTP composition boundary. The process entry point owns
// concrete persistence construction; routing only wires injected handlers.
type Dependencies struct {
	Domains        *tenancyhttp.Handler
	PublicIDs      PublicIDResolver
	ResolveDomain  gin.HandlerFunc
	Auth           *identityhttp.AuthHandler
	Health         *HealthHandler
	Submissions    *submissionhandler.SubmissionHandler
	Problems       *problemhttp.ProblemHandler
	Contests       *contesthttp.ContestHandler
	Editorials     *contenthttp.EditorialHandler
	Discussions    *contenthttp.DiscussionHandler
	AdminProblems  *problemhttp.AdminProblemHandler
	AdminPackages  *authoringhandler.PackageHandler
	ProblemSets    *sethttp.SetHandler
	Console        *consolehttp.ConsoleHandler
	Builds         *authoringhandler.BuildHandler
	Profiles       *profilehttp.ProfileHandler
	Judge          *judgehttp.JudgeHandler
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
		problemhttp.RegisterRoutes(scope, deps.Problems, deps.AdminProblems, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		authoringhandler.RegisterRoutes(scope, deps.AdminPackages, deps.RequireAuth, resourceScope...)
		sethttp.RegisterRoutes(scope, deps.ProblemSets, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		deps.Contests.RegisterRoutes(scope, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		deps.Submissions.RegisterRoutes(scope, deps.RequireAuth, resourceScope...)
		contenthttp.RegisterRoutes(scope, deps.Editorials, deps.Discussions, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
		deps.Profiles.RegisterRoutes(scope, append([]gin.HandlerFunc{deps.OptionalAuth}, resourceScope...)...)
	}
	consolehttp.RegisterRoutes(api, deps.Console, deps.OptionalAuth, deps.RequireAuth, deps.RequireAdmin, resourceScope...)
	if deps.ResolveDomain != nil {
		consolehttp.RegisterResourceRoutes(api.Group("/domains/:domain"), deps.Console, deps.OptionalAuth, deps.RequireAuth, resourceScope...)
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
