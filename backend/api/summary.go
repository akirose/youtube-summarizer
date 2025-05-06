package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/akirose/youtube-summarizer/auth"
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

// InitSummaryModule은 요약 기능과 관련된 모든 초기화 작업을 수행합니다.
func InitSummaryModule() error {
	// 캐시 초기화
	if err := InitCache(); err != nil {
		return err
	}

	// 사용자 요약 디렉토리 초기화
	if err := models.InitUserSummaryDirectory(); err != nil {
		return err
	}

	return nil
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

	// auth 패키지의 GetSessionUser를 사용하여 사용자 정보 조회
	userInfo, authenticated := auth.GetSessionUser(c)
	if !authenticated || userInfo == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "인증된 사용자 정보를 찾을 수 없습니다.",
		})
		return
	}

	// 안전하게 사용자 ID 추출
	userID := userInfo.ID

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
			// 캐시에서 발견되었지만, 사용자 요약 기록은 추가해야 합니다.
			if err := models.AddUserSummary(userID, videoID, cachedItem.Title); err != nil {
				// 오류 기록하지만 요청은 계속 진행
				// TODO: 제대로 된 로깅 구현
			}

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

	// 전역 캐시와 사용자 요약에 결과 저장
	if summaryCache != nil {
		if err := summaryCache.AddUserSummaryToCache(userID, videoID, videoInfo.Title, summary, nil); err != nil {
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

// GetUserRecentSummariesHandler는 사용자의 최근 15개 요약을 가져오는 API 핸들러입니다.
func GetUserRecentSummariesHandler(c *gin.Context) {
	// auth 패키지의 GetSessionUser를 사용하여 사용자 정보 조회
	userInfo, authenticated := auth.GetSessionUser(c)
	if !authenticated || userInfo == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "인증된 사용자 정보를 찾을 수 없습니다.",
		})
		return
	}

	// 안전하게 사용자 ID 추출
	userID := userInfo.ID

	// 사용자의 최근 요약을 가져옵니다.
	summaries, err := models.GetRecentUserSummaries(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "사용자 요약을 가져오는데 실패했습니다: " + err.Error(),
		})
		return
	}

	// 응답 반환
	c.JSON(http.StatusOK, summaries)
}
