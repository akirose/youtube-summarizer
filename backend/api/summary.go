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
	VideoID    string            `json:"videoId"`
	Title      string            `json:"title"`
	Summary    string            `json:"summary"`
	Timestamps []models.Timestamp `json:"timestamps"`
	Cached     bool              `json:"cached"`
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

// Convert TimestampInfo to Timestamp
func convertTimestamps(timestamps []services.TimestampInfo) []models.Timestamp {
	result := make([]models.Timestamp, len(timestamps))
	for i, ts := range timestamps {
		result[i] = models.Timestamp{
			Time: ts.Time,
			Text: ts.Text,
		}
	}
	return result
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

	// Get video transcript
	transcript, err := services.GetTranscript(videoID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get video transcript: " + err.Error(),
		})
		return
	}

	// Format transcript for summarization
	formattedTranscript := services.GetFormattedTranscript(transcript)

	// Generate summary using OpenAI
	summary, timestampsInfo, err := services.SummarizeTranscript(formattedTranscript)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate summary: " + err.Error(),
		})
		return
	}

	// Convert timestamp info to model timestamps
	timestamps := convertTimestamps(timestampsInfo)

	// Cache the result
	if summaryCache != nil {
		if err := summaryCache.Set(videoID, videoInfo.Title, summary, timestamps); err != nil {
			// Just log the error, don't fail the request
			// Using default logger since we don't have a global logger yet
			// TODO: Implement proper logging
			// global.Logger.Printf("Failed to cache summary: %v", err)
		}
	}

	// Return response
	c.JSON(http.StatusOK, SummaryResponse{
		VideoID:    videoID,
		Title:      videoInfo.Title,
		Summary:    summary,
		Timestamps: timestamps,
		Cached:     false,
	})
}
