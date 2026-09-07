package handler

import "github.com/gin-gonic/gin"

// RegisterRoutes wires editorials and discussions. Reads use optional
// authentication because the spoiler gate, draft visibility and "did I vote"
// all depend on who is asking.
func RegisterRoutes(
	api *gin.RouterGroup,
	editorials *EditorialHandler,
	discussions *DiscussionHandler,
	optionalAuth gin.HandlerFunc,
	requireAuth gin.HandlerFunc,
	resolveIDs ...gin.HandlerFunc,
) {
	editorialPublic := api.Group("/editorials")
	editorialPublic.Use(optionalAuth)
	editorialPublic.Use(resolveIDs...)
	editorialPublic.GET("", editorials.List)
	editorialPublic.GET("/:id", editorials.Get)
	editorialPublic.GET("/:id/discussions", discussions.ListByEditorial)

	discussionPublic := api.Group("/problems")
	discussionPublic.Use(optionalAuth)
	discussionPublic.Use(resolveIDs...)
	discussionPublic.GET("/:id/discussions", discussions.ListByProblem)

	editorialAuthed := api.Group("/editorials")
	editorialAuthed.Use(requireAuth)
	editorialAuthed.Use(resolveIDs...)
	editorialAuthed.POST("", editorials.Create)
	editorialAuthed.PUT("/:id", editorials.Update)
	editorialAuthed.DELETE("/:id", editorials.Delete)
	editorialAuthed.POST("/:id/vote", editorials.Vote)
	editorialAuthed.POST("/:id/discussions", discussions.CreateEditorialPost)

	discussionAuthed := api.Group("")
	discussionAuthed.Use(requireAuth)
	discussionAuthed.Use(resolveIDs...)
	discussionAuthed.POST("/problems/:id/discussions", discussions.CreateProblemPost)
	discussionAuthed.PUT("/discussions/:postId", discussions.Update)
	discussionAuthed.DELETE("/discussions/:postId", discussions.Delete)
}
