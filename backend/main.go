package main

import (
	"log"
	"os"

	"github.com/akirose/youtube-summarizer/api"
	"github.com/akirose/youtube-summarizer/auth"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found")
	}

	// Initialize cache
	if err := api.InitCache(); err != nil {
		log.Printf("Warning: Failed to initialize cache: %v\n", err)
	}

	// Initialize auth
	auth.InitAuth()

	// Set default port if not specified
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create Gin router
	router := gin.Default()

	// CORS 미들웨어 설정
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Load HTML templates
	router.LoadHTMLGlob("templates/*")

	// Serve static files from frontend directory
	router.StaticFile("/", "../frontend/index.html")
	router.Static("/css", "../frontend/css")
	router.Static("/js", "../frontend/js")
	router.Static("/img", "../frontend/img")

	// Auth routes
	authGroup := router.Group("/auth")
	{
		authGroup.GET("/google", auth.GoogleLoginHandler)
		authGroup.GET("/google/callback", auth.GoogleCallbackHandler)
		authGroup.POST("/logout", auth.LogoutHandler)
	}

	// User routes (인증 필요)
	userGroup := router.Group("/user")
	userGroup.Use(auth.IsAuthenticated())
	{
		userGroup.GET("/info", getUserInfo)
	}

	// API routes
	apiGroup := router.Group("/api")
	{
		// 요약 요청은 인증이 필요
		apiGroup.POST("/summary", auth.IsAuthenticated(), api.HandleSummaryRequest)

		// 최근 요약 목록도 인증이 필요
		apiGroup.GET("/recent-summaries", auth.IsAuthenticated(), api.GetRecentSummariesHandler)
	}

	// Start server
	log.Printf("Server starting on port %s...\n", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

// 현재 사용자 정보를 반환하는 핸들러
func getUserInfo(c *gin.Context) {
	userInfo, authenticated := auth.GetSessionUser(c)
	if !authenticated {
		c.JSON(401, gin.H{"error": "Not authenticated"})
		return
	}

	c.JSON(200, gin.H{
		"user":          userInfo,
		"authenticated": true,
	})
}
