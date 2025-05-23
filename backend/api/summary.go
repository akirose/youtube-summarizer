package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	VideoID    string                    `json:"videoId"`
	Title      string                    `json:"title"`
	Summary    string                    `json:"summary"`
	Timestamps []models.Timestamp        `json:"timestamps"`
	Transcript []services.TranscriptItem `json:"transcript,omitempty"`
	Cached     bool                      `json:"cached"`
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

// 사용자의 API 키를 Authorization 헤더에서 추출합니다
func extractAPIKeyFromHeader(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	// "Bearer " 접두사 확인 및 제거
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}

	return strings.TrimPrefix(authHeader, "Bearer ")
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

	// Authorization 헤더에서 사용자 API 키 추출
	userAPIKey := extractAPIKeyFromHeader(c)

	// API 키 사용 가능 여부 확인
	if userAPIKey == "" {
		// 사용자가 API 키를 제공하지 않은 경우 서버 키 사용 가능한지 확인
		policy := services.GetAPIKeyPolicy()
		if !policy.CanUseServerKey(userID) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "API 키가 필요합니다. 설정에서 OpenAI API 키를 설정해주세요.",
			})
			return
		}
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
			// 캐시에서 발견되었지만, 사용자 요약 기록은 추가해야 합니다.
			if err := models.AddUserSummary(userID, videoID, cachedItem.Title); err != nil {
				// 오류 기록하지만 요청은 계속 진행
				// TODO: 제대로 된 로깅 구현
			}

			// If there's transcript data in the cache (may not exist in older cache entries)
			var transcript []services.TranscriptItem = cachedItem.Transcript

			// Get fresh transcript data if needed (not in cache)
			if len(transcript) == 0 {
				chunks, err := services.GetTranscript(videoID, 0) // Here we use 0 to get all items in one chunk
				if err == nil && len(chunks) > 0 {
					transcript = chunks[0] // Take the first chunk which should contain all items
					summaryCache.Set(videoID, cachedItem.Title, cachedItem.Summary, nil, transcript)
				}
			}

			// Return cached response with transcript if available
			c.JSON(http.StatusOK, SummaryResponse{
				VideoID:    videoID,
				Title:      cachedItem.Title,
				Summary:    cachedItem.Summary,
				Timestamps: cachedItem.Timestamps,
				Transcript: MergeTranscript(transcript),
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
	chunks, err := services.GetTranscript(videoID, 400.0) // Chunk size of 800 seconds
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get video transcript: " + err.Error(),
		})
		return
	}

	// Summarize chunks and combine into final summary
	summary, err := services.SummarizeChunks(chunks, userAPIKey, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to summarize transcript chunks: " + err.Error(),
		})
		return
	}

	// Get transcript data for the transcript tab
	var transcriptItems []services.TranscriptItem

	// Combine all chunks into a flat array of transcript items
	if len(chunks) > 0 {
		// Combine all chunks into a flat array of transcript items
		for _, chunk := range chunks {
			transcriptItems = append(transcriptItems, chunk...)
		}

		// Sort by timestamp to ensure correct order
		services.SortTranscriptItemsByTime(transcriptItems)
	}

	// 전역 캐시와 사용자 요약에 결과 저장 (트랜스크립트 포함)
	if summaryCache != nil {
		if err := summaryCache.AddUserSummaryToCache(userID, videoID, videoInfo.Title, summary, nil, transcriptItems); err != nil {
			// Log the error but don't fail the request
			// TODO: Implement proper logging
		}
	}

	// Return response with both summary and transcript
	c.JSON(http.StatusOK, SummaryResponse{
		VideoID:    videoID,
		Title:      videoInfo.Title,
		Summary:    summary,
		Timestamps: nil, // Timestamps are not used in this flow
		Transcript: MergeTranscript(transcriptItems),
		Cached:     false,
	})
}

func MergeTranscript(transcript []services.TranscriptItem) []services.TranscriptItem {
	if len(transcript) == 0 {
		return transcript
	}

	var result []services.TranscriptItem
	var currentItem services.TranscriptItem
	const intervalSeconds float64 = 15.0

	// Initialize with the first item
	currentItem = transcript[0]

	for i := 1; i < len(transcript); i++ {
		// If the next item starts within 15 seconds of the current item's start time
		if transcript[i].Start-currentItem.Start < intervalSeconds {
			// Append text to the current item
			currentItem.Text += transcript[i].Text
			// Keep the duration updating to the last item's end time
			currentItem.Duration = transcript[i].Start + transcript[i].Duration - currentItem.Start
		} else {
			// Current interval is complete, add to result and start a new interval
			result = append(result, currentItem)
			currentItem = transcript[i]
		}
	}

	// Add the last item
	result = append(result, currentItem)

	return result
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
