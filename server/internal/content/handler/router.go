package handler

import "github.com/gin-gonic/gin"

func RegisterRoutes(
	api *gin.RouterGroup,
	editorials *EditorialHandler,
	discussions *DiscussionHandler,
	requireAuth gin.HandlerFunc,
) {
	editorialPublic := api.Group("/editorials")
	editorialPublic.GET("", editorials.ListByProblem)
	editorialPublic.GET("/:id", editorials.Get)
	editorialPublic.GET("/:id/discussions", discussions.ListByEditorial)

	discussionPublic := api.Group("/problems")
	discussionPublic.GET("/:id/discussions", discussions.ListByProblem)

	editorialAuthed := api.Group("/editorials")
	editorialAuthed.Use(requireAuth)
	editorialAuthed.POST("", editorials.Create)
	editorialAuthed.POST("/:id/discussions", discussions.CreateEditorialPost)

	discussionAuthed := api.Group("")
	discussionAuthed.Use(requireAuth)
	discussionAuthed.POST("/problems/:id/discussions", discussions.CreateProblemPost)
	discussionAuthed.DELETE("/discussions/:id", discussions.Delete)
}
