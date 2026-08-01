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
	authed := r.Group("/api")
	authed.Use(RequireAuth())
	{
		authed.POST("/submissions", subH.Submit)
		authed.GET("/submissions", subH.List)
		authed.GET("/submissions/:id", subH.Get)
	}

	// 需 admin
	admin := r.Group("/api")
	admin.Use(RequireAuth(), RequireAdmin())
	{
		admin.POST("/submissions/:id/rejudge", subH.Rejudge)
		// 出题相关(M2 填充)
		admin.POST("/problems", func(c *gin.Context) {
			c.JSON(http.StatusNotImplemented, gin.H{"error": "problem authoring lands in M2"})
		})
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
