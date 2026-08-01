package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vertex-oj/web/internal/store"
)

// Router 组装所有 API 路由。
func Router(db *store.DB) *gin.Engine {
	r := gin.Default()

	// CORS(前端开发服务器跨域访问)
	r.Use(corsMiddleware())

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	users := store.NewUserStore(db)
	authH := NewAuthHandler(users)

	// 认证(公开)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/register", authH.Register)
		authGroup.POST("/login", authH.Login)
		authGroup.GET("/me", RequireAuth(), authH.Me)
	}

	// 需登录
	submissions := store.NewSubmissionStore(db)
	problems := store.NewProblemStore(db)
	subH := NewSubmissionHandler(submissions, problems, nil)
	problemH := NewProblemHandler(problems)
	authed := r.Group("/api")
	authed.Use(RequireAuth())
	{
		authed.POST("/submissions", subH.Submit)
		authed.GET("/submissions", subH.List)
		authed.GET("/submissions/:id", subH.Get)
	}

	// 题目(公开列表 + 详情;可见性在 handler 内校验)
	pub := r.Group("/api")
	{
		pub.GET("/problems", problemH.List)
		pub.GET("/problems/:id", problemH.Get)
	}

	// 需 admin
	adminStore := store.NewProblemAdminStore(db, testdataRoot())
	adminProblemH := NewAdminProblemHandler(adminStore, problems)
	admin := r.Group("/api/admin")
	admin.Use(RequireAuth(), RequireAdmin())
	{
		admin.GET("/problems", adminProblemH.List)
		admin.POST("/problems", adminProblemH.Create)
		admin.GET("/problems/:id", adminProblemH.Get)
		admin.PUT("/problems/:id", adminProblemH.Update)
		admin.DELETE("/problems/:id", adminProblemH.Delete)
		admin.POST("/problems/:id/testdata", adminProblemH.UploadTestdata)

		admin.POST("/submissions/:id/rejudge", subH.Rejudge)
	}

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
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
