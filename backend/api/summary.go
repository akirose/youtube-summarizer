package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/akirose/youtube-summarizer/models"
	"github.com/akirose/youtube-summarizer/services"
	"github.com/gin-gonic/gin"
)

// SummaryRequest represents the request for a video summary
type SummaryRequest struct {
	URL string `json:"url" binding:"required"`
}

// SummaryResponse represents the response with the video summary
type SummaryResponse struct {
	VideoID    string             `json:"videoId"`
	Title      string             `json:"title"`
	Summary    string             `json:"summary"`
	Timestamps []models.Timestamp `json:"timestamps"`
	Cached     bool               `json:"cached"`
}

// Global cache instance
var summaryCache *models.SummaryCache

// InitCache initializes the summary cache
func InitCache() error {
	// Get cache directory
	cacheDir := os.Getenv("CACHE_DIR")
	if cacheDir == "" {
		// Default to "cache" directory in the current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		cacheDir = filepath.Join(cwd, "cache")
	}

	// Create cache
	var err error
	summaryCache, err = models.NewSummaryCache(cacheDir)
	return err
}

// HandleSummaryRequest processes a request to summarize a YouTube video
func HandleSummaryRequest(c *gin.Context) {
	var request SummaryRequest

	// Bind request body to struct
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request: " + err.Error(),
		})
		return
	}

	// Extract video ID from URL
	videoID, err := services.GetVideoID(request.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid YouTube URL: " + err.Error(),
		})
		return
	}

	// Check cache first
	if summaryCache != nil {
		if cachedItem, found := summaryCache.Get(videoID); found {
			// Return cached response
			c.JSON(http.StatusOK, SummaryResponse{
				VideoID:    videoID,
				Title:      cachedItem.Title,
				Summary:    cachedItem.Summary,
				Timestamps: cachedItem.Timestamps,
				Cached:     true,
			})
			return
		}
	}

	// Get video info (title, etc.)
	videoInfo, err := services.GetVideoInfo(videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get video info: " + err.Error(),
		})
		return
	}

	// Get video transcript in chunks
	chunks, err := services.GetTranscript(videoID, 800.0) // Chunk size of 800 seconds
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get video transcript: " + err.Error(),
		})
		return
	}

	// Summarize chunks and combine into final summary
	summary, err := services.SummarizeChunks(chunks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to summarize transcript chunks: " + err.Error(),
		})
		return
	}

	// Cache the result
	if summaryCache != nil {
		if err := summaryCache.Set(videoID, videoInfo.Title, summary, nil); err != nil {
			// Log the error but don't fail the request
			// TODO: Implement proper logging
		}
	}

	// Return response
	c.JSON(http.StatusOK, SummaryResponse{
		VideoID:    videoID,
		Title:      videoInfo.Title,
		Summary:    summary,
		Timestamps: nil, // Timestamps are not used in this flow
		Cached:     false,
	})
}

// GetRecentSummariesHandler handles requests to fetch the last 10 video summaries
func GetRecentSummariesHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")

	// Fetch the recent 10 video summaries
	summaries := models.GetRecentVideoSummaries()

	// Respond with the summaries in JSON format
	c.JSON(http.StatusOK, summaries)
}
