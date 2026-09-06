package handler

import "github.com/gin-gonic/gin"

// RegisterRoutes wires the permission-checked package workspace. It extends the
// existing /admin/problems group, so problem metadata and its package stay one
// resource tree.
func RegisterRoutes(api *gin.RouterGroup, packages *PackageHandler, requireAuth gin.HandlerFunc, resolveIDs ...gin.HandlerFunc) {
	admin := api.Group("/admin")
	admin.Use(requireAuth)
	admin.Use(resolveIDs...)
	admin.GET("/package-templates", packages.Templates)

	problems := admin.Group("/problems/:id")
	problems.GET("/package", packages.Workspace)

	problems.GET("/statements", packages.ListStatements)
	problems.PUT("/statements/:language", packages.SaveStatement)
	problems.DELETE("/statements/:language", packages.DeleteStatement)
	problems.POST("/statements/:language/preview", packages.PreviewStatement)

	problems.GET("/files", packages.ListFiles)
	problems.PUT("/files", packages.SaveFile)
	problems.GET("/files/:fileId", packages.GetFile)
	problems.DELETE("/files/:fileId", packages.DeleteFile)

	problems.GET("/tests", packages.ListTests)
	problems.POST("/tests", packages.CreateTest)
	problems.PUT("/tests/:testId", packages.UpdateTest)
	problems.DELETE("/tests/:testId", packages.DeleteTest)
	problems.POST("/tests/:testId/move", packages.MoveTest)

	problems.GET("/builds", packages.ListBuilds)
	problems.POST("/builds", packages.StartBuild)
	problems.GET("/builds/:buildId", packages.GetBuild)
	problems.POST("/builds/:buildId/cancel", packages.CancelBuild)
}

// RegisterInternalRoutes wires the build worker protocol next to the judge
// worker protocol so both share one credential and one base URL.
func (h *BuildHandler) RegisterInternalRoutes(internal *gin.RouterGroup, requireJudge gin.HandlerFunc) {
	builds := internal.Group("/judge/v1/builds")
	builds.Use(requireJudge)
	builds.POST("/claim", h.Claim)
	builds.POST("/:buildId/progress", h.Progress)
	builds.POST("/:buildId/package", h.UploadPackage)
	builds.PUT("/:buildId/result", h.Complete)
}
