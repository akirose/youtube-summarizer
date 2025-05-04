package main

import (
	"log"
	"os"

	"github.com/akirose/youtube-summarizer/api"
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

	// Set default port if not specified
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create Gin router
	router := gin.Default()

	// Serve static files from frontend directory
	router.StaticFile("/", "../frontend/index.html")
	router.Static("/css", "../frontend/css")
	router.Static("/js", "../frontend/js")
	router.Static("/img", "../frontend/img")

	// API routes
	apiGroup := router.Group("/api")
	{
		apiGroup.POST("/summary", api.HandleSummaryRequest)
		apiGroup.GET("/recent-summaries", api.GetRecentSummariesHandler)
	}

	// Start server
	log.Printf("Server starting on port %s...\n", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
