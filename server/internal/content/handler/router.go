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
) {
	editorialPublic := api.Group("/editorials")
	editorialPublic.Use(optionalAuth)
	editorialPublic.GET("", editorials.List)
	editorialPublic.GET("/:id", editorials.Get)
	editorialPublic.GET("/:id/discussions", discussions.ListByEditorial)

	discussionPublic := api.Group("/problems")
	discussionPublic.Use(optionalAuth)
	discussionPublic.GET("/:id/discussions", discussions.ListByProblem)

	contestPublic := api.Group("/contests")
	contestPublic.Use(optionalAuth)
	contestPublic.GET("/:id/discussions", discussions.ListByContest)

	editorialAuthed := api.Group("/editorials")
	editorialAuthed.Use(requireAuth)
	editorialAuthed.POST("", editorials.Create)
	editorialAuthed.PUT("/:id", editorials.Update)
	editorialAuthed.DELETE("/:id", editorials.Delete)
	editorialAuthed.POST("/:id/vote", editorials.Vote)
	editorialAuthed.POST("/:id/discussions", discussions.CreateEditorialPost)

	discussionAuthed := api.Group("")
	discussionAuthed.Use(requireAuth)
	discussionAuthed.POST("/problems/:id/discussions", discussions.CreateProblemPost)
	discussionAuthed.POST("/contests/:id/discussions", discussions.CreateContestPost)
	discussionAuthed.PUT("/discussions/:postId", discussions.Update)
	discussionAuthed.DELETE("/discussions/:postId", discussions.Delete)
}
