package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/akirose/youtube-summarizer/auth"
	"github.com/akirose/youtube-summarizer/models"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/akirose/youtube-summarizer/services"
	"github.com/gin-gonic/gin"
)

// Global map for SSE client channels (UserID -> channel)
var clientChannels = make(map[string]chan []byte)
var clientChannelsMutex = &sync.RWMutex{}

// Global map for active video summarization jobs (VideoID -> list of UserIDs)
var activeVideoJobs = make(map[string][]string)
var activeVideoJobsMutex = &sync.RWMutex{}

// SummarizationJob defines the structure for a video summarization job
type SummarizationJob struct {
	VideoID  string
	UserID   string
	APIKey   string // User's API key, if provided
	URL      string // Original URL, mainly for context if needed later
	IsSSE    bool   // Flag to indicate if this job is for SSE
	ClientID string // SSE Client ID
}

// Global job queue
var jobQueue chan SummarizationJob

const defaultNumWorkers = 3
const jobQueueCapacity = 100

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

	// Initialize job queue
	jobQueue = make(chan SummarizationJob, jobQueueCapacity)

	// Initialize SSE client channels map
	clientChannels = make(map[string]chan []byte)

	// Initialize active video jobs map
	activeVideoJobs = make(map[string][]string)

	// Start worker pool
	numWorkersStr := os.Getenv("NUM_SUMMARY_WORKERS")
	numWorkers, err := strconv.Atoi(numWorkersStr)
	if err != nil || numWorkers <= 0 {
		log.Printf("Warning: Invalid or missing NUM_SUMMARY_WORKERS environment variable ('%s'). Defaulting to %d workers.", numWorkersStr, defaultNumWorkers)
		numWorkers = defaultNumWorkers
	}
	startWorkerPool(numWorkers, jobQueue) // Assuming startWorkerPool has its own "Worker X starting" logs
	log.Printf("Info: Summarization worker pool configured with %d workers. Job queue capacity: %d.", numWorkers, jobQueueCapacity)

	return nil
}

// startWorkerPool launches worker goroutines.
func startWorkerPool(numWorkers int, queue chan SummarizationJob) {
	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			log.Printf("Info: Worker %d starting.", workerID)
			// Outer defer for the worker goroutine itself
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Error: Worker %d encountered a critical panic: %v. Worker is stopping.", workerID, r)
					// In a production system, consider metrics/alerting for this.
				} else {
					log.Printf("Info: Worker %d stopping.", workerID)
				}
			}()

			for job := range queue {
				// Inner func and defer/recover for per-job panic safety
				func(currentJob SummarizationJob) {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("Error: Worker %d: Panic during processing of VideoID: %s, UserID: %s. Panic: %v", workerID, currentJob.VideoID, currentJob.UserID, r)
							// Notify subscribers of the error due to panic
							errorData := gin.H{"videoId": currentJob.VideoID, "error": "Server error during summarization."}
							jsonData, _ := json.Marshal(errorData) // Error here is unlikely
							sseMessage := []byte(fmt.Sprintf("event: summary_error\ndata: %s\n\n", string(jsonData)))

							activeVideoJobsMutex.Lock()
							subscribers, ok := activeVideoJobs[currentJob.VideoID]
							if ok {
								delete(activeVideoJobs, currentJob.VideoID) // Clean up active job
							}
							activeVideoJobsMutex.Unlock()

							for _, subscriberUserID := range subscribers {
								sendSSEMessage(subscriberUserID, sseMessage)
							}
						}
					}()

					log.Printf("Info: Worker %d: Picked up job for VideoID: %s (Original UserID: %s)", workerID, currentJob.VideoID, currentJob.UserID)
					summaryResp, err := processSummarizationJob(currentJob)

					// After processing, get all subscribed users for this videoID
				activeVideoJobsMutex.Lock()
				subscribers, ok := activeVideoJobs[job.VideoID]
				if ok {
					delete(activeVideoJobs, job.VideoID) // Remove job from active list
				}
				activeVideoJobsMutex.Unlock()

					activeVideoJobsMutex.Lock()
					subscribers, subscribersFound := activeVideoJobs[currentJob.VideoID]
					if subscribersFound {
						delete(activeVideoJobs, currentJob.VideoID)
					}
					activeVideoJobsMutex.Unlock()

					if !subscribersFound && err == nil { 
						log.Printf("Warning: Worker %d: No subscribers found for VideoID: %s (Original UserID: %s) after processing. This might indicate a state issue or race condition if the job was meant to have subscribers.", workerID, currentJob.VideoID, currentJob.UserID)
					}

					for _, subscriberUserID := range subscribers {
						if err != nil {
							log.Printf("Info: Worker %d: Notifying subscriber %s of error for VideoID %s. Error: %v", workerID, subscriberUserID, currentJob.VideoID, err)
							errorData := gin.H{"videoId": currentJob.VideoID, "error": err.Error()}
							jsonData, _ := json.Marshal(errorData)
							sseMessage := []byte(fmt.Sprintf("event: summary_error\ndata: %s\n\n", string(jsonData)))
							sendSSEMessage(subscriberUserID, sseMessage)
						} else if summaryResp != nil {
							log.Printf("Info: Worker %d: Notifying subscriber %s of success for VideoID %s.", workerID, subscriberUserID, currentJob.VideoID)
							jsonData, jsonErr := json.Marshal(summaryResp)
							if jsonErr != nil {
								log.Printf("Error: Worker %d: Failed to marshal summary response for SSE (Subscriber: %s, VideoID: %s): %v", workerID, subscriberUserID, currentJob.VideoID, jsonErr)
								errorData := gin.H{"videoId": currentJob.VideoID, "error": "Internal server error: Failed to serialize summary data."}
								errorJson, _ := json.Marshal(errorData)
								sseMessage := []byte(fmt.Sprintf("event: summary_error\ndata: %s\n\n", string(errorJson)))
								sendSSEMessage(subscriberUserID, sseMessage)
							} else {
								sseMessage := []byte(fmt.Sprintf("event: summary_complete\ndata: %s\n\n", string(jsonData)))
								sendSSEMessage(subscriberUserID, sseMessage)
							}
						}
					}
					if err != nil {
						log.Printf("Info: Worker %d: Finished job for VideoID: %s (Original UserID: %s) with error: %v", workerID, currentJob.VideoID, currentJob.UserID, err)
					} else {
						log.Printf("Info: Worker %d: Finished job successfully for VideoID: %s (Original UserID: %s)", workerID, currentJob.VideoID, currentJob.UserID)
					}
				}(job) // Pass job as an argument to the inner func
			}
		}(i + 1)
	}
}

// sendSSEMessage sends a message to a specific user's SSE channel if it exists.
// It is non-blocking to prevent workers from getting stuck.
func sendSSEMessage(userID string, message []byte) {
	clientChannelsMutex.RLock()
	clientChan, ok := clientChannels[userID]
	clientChannelsMutex.RUnlock()

	msgPreview := string(message)
	if len(msgPreview) > 100 { // Limit preview length
		msgPreview = msgPreview[:100] + "..."
	}

	if ok {
		select {
		case clientChan <- message:
			log.Printf("Info: Sent SSE message to UserID %s (preview: %s)", userID, msgPreview)
		default:
			log.Printf("Warning: SSE channel for UserID %s is full. Message dropped (preview: %s)", userID, msgPreview)
		}
	} else {
		log.Printf("Info: No active SSE channel for UserID %s. Message not sent (preview: %s)", userID, msgPreview)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// processSummarizationJob handles the actual video summarization.
func processSummarizationJob(job SummarizationJob) (*SummaryResponse, error) {
	log.Printf("Info: Worker: Processing job for VideoID: %s (Original UserID: %s)", job.VideoID, job.UserID)

	// This initial cache check can be useful if a job was queued, but by the time a worker picks it up,
	// another worker (or a direct request for the same video) has already populated the cache.
	if summaryCache != nil {
		if cachedItem, found := summaryCache.Get(job.VideoID); found {
			log.Printf("Info: Worker: VideoID %s (Original UserID: %s) found in cache by worker. Ensuring user summary and returning.", job.VideoID, job.UserID)
			// Ensure user summary is recorded for the *original* requester of this job.
			if err := models.AddUserSummary(job.UserID, job.VideoID, cachedItem.Title); err != nil {
				log.Printf("Warning: Worker: VideoID %s, UserID %s: Error adding user summary in worker (cache hit scenario): %v", job.VideoID, job.UserID, err)
			}

			var transcriptToReturn []services.TranscriptItem = cachedItem.Transcript
			if len(transcriptToReturn) == 0 {
				freshChunks, errTr := services.GetTranscript(job.VideoID, 0)
				if errTr == nil && len(freshChunks) > 0 {
					transcriptToReturn = freshChunks[0]
					if cacheErr := summaryCache.Set(job.VideoID, cachedItem.Title, cachedItem.Summary, cachedItem.Timestamps, transcriptToReturn); cacheErr != nil {
						log.Printf("Warning: Worker: VideoID %s: Failed to update cache with transcript (worker cache hit): %v", job.VideoID, cacheErr)
					}
				} else if errTr != nil {
					log.Printf("Warning: Worker: VideoID %s: Failed to fetch transcript in worker (cache hit, transcript miss): %v", job.VideoID, errTr)
				}
			}
			return &SummaryResponse{
				VideoID:    job.VideoID,
				Title:      cachedItem.Title,
				Summary:    cachedItem.Summary,
				Timestamps: cachedItem.Timestamps,
				Transcript: MergeTranscript(transcriptToReturn),
				Cached:     true, // Indicate it was served from cache by the worker.
			}, nil
		}
	}

	videoInfo, err := services.GetVideoInfo(job.VideoID)
	if err != nil {
		log.Printf("Error: Worker: VideoID %s, UserID %s: Failed to get video info: %v", job.VideoID, job.UserID, err)
		return nil, fmt.Errorf("failed to get video info for VideoID %s: %w", job.VideoID, err)
	}

	chunks, err := services.GetTranscript(job.VideoID, 400.0)
	if err != nil {
		log.Printf("Error: Worker: VideoID %s, UserID %s: Failed to get video transcript: %v", job.VideoID, job.UserID, err)
		return nil, fmt.Errorf("failed to get transcript for VideoID %s: %w", job.VideoID, err)
	}

	summaryText, err := services.SummarizeChunks(chunks, job.APIKey, job.UserID)
	if err != nil {
		log.Printf("Error: Worker: VideoID %s, UserID %s: Failed to summarize transcript chunks: %v", job.VideoID, job.UserID, err)
		return nil, fmt.Errorf("failed to summarize transcript for VideoID %s: %w", job.VideoID, err)
	}

	var transcriptItems []services.TranscriptItem
	if len(chunks) > 0 {
		for _, chunk := range chunks {
			transcriptItems = append(transcriptItems, chunk...)
		}
		services.SortTranscriptItemsByTime(transcriptItems)
	}

	if summaryCache != nil {
		// job.UserID is the initial requester. AddUserSummaryToCache also adds to their list.
		if err := summaryCache.AddUserSummaryToCache(job.UserID, job.VideoID, videoInfo.Title, summaryText, nil, transcriptItems); err != nil {
			log.Printf("Warning: Worker: VideoID %s, UserID %s: Error saving summary to cache: %v. Processing continues, but result may not be cached.", job.VideoID, job.UserID, err)
			// Not returning an error here as summary was generated, just caching failed.
		}
	}

	log.Printf("Info: Worker: Successfully processed and cached summary for VideoID %s (Original UserID: %s)", job.VideoID, job.UserID)

	// This response is what would eventually be sent via SSE.
	// For now, it's logged by the worker.
	return &SummaryResponse{
		VideoID:    job.VideoID,
		Title:      videoInfo.Title,
		Summary:    summaryText,
		Timestamps: nil, // Timestamps are not used in this new flow directly in response
		Transcript: MergeTranscript(transcriptItems),
		Cached:     false, // It's newly generated
	}, nil
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid YouTube URL: " + err.Error()})
		return
	}

	// Check cache first
	if summaryCache != nil {
		if cachedItem, found := summaryCache.Get(videoID); found {
			log.Printf("Info: HandleSummaryRequest: Cache hit for VideoID: %s, requesting UserID: %s.", videoID, userID)
			// Ensure this user has this summary in their list, even if it was cached by another user or system process
			if err := models.AddUserSummary(userID, videoID, cachedItem.Title); err != nil {
				log.Printf("Warning: HandleSummaryRequest (Cache Hit): UserID %s, VideoID %s: Failed to add user summary: %v", userID, videoID, err)
			}

			var transcript []services.TranscriptItem = cachedItem.Transcript
			if len(transcript) == 0 {
				chunks, errTr := services.GetTranscript(videoID, 0)
				if errTr == nil && len(chunks) > 0 {
					transcript = chunks[0]
					summaryCache.Set(videoID, cachedItem.Title, cachedItem.Summary, nil, transcript) // Update cache with transcript
				} else if errTr != nil {
					log.Printf("Error fetching transcript for cached item %s: %v", videoID, errTr)
				}
			}

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

	// Deduplication logic for active jobs
	activeVideoJobsMutex.Lock()
	subscribers, isJobActive := activeVideoJobs[videoID]
	if isJobActive {
		alreadySubscribed := false
		for _, subUserID := range subscribers {
			if subUserID == userID {
				alreadySubscribed = true
				break
			}
		}
		if !alreadySubscribed {
			activeVideoJobs[videoID] = append(subscribers, userID)
			log.Printf("Info: HandleSummaryRequest: VideoID %s already being processed/queued. Added UserID %s to subscribers list.", videoID, userID)
		} else {
			log.Printf("Info: HandleSummaryRequest: VideoID %s already being processed/queued. UserID %s is already a subscriber.", videoID, userID)
		}
		activeVideoJobsMutex.Unlock()
		c.JSON(http.StatusAccepted, gin.H{
			"message":  "Summarization for this video is already in progress or queued. You will be notified upon completion.",
			"video_id": videoID,
		})
		return
	}

	activeVideoJobs[videoID] = []string{userID} // Register new job with this user as the first subscriber
	activeVideoJobsMutex.Unlock()
	log.Printf("Info: HandleSummaryRequest: New summarization request for VideoID %s by UserID %s. Registered and attempting to queue.", videoID, userID)
	job := SummarizationJob{
		VideoID:  videoID,
		UserID:   userID, // UserID here is the initial requester. Worker will use VideoID to get all subscribers.
		APIKey:   userAPIKey,
		URL:      request.URL,
		IsSSE:    true,
		ClientID: "",
	}

	select {
	case jobQueue <- job:
		log.Printf("Job queued for VideoID: %s by UserID: %s", videoID, userID)
		c.JSON(http.StatusAccepted, gin.H{
			"message":  "Summarization request received and queued. You will be notified upon completion.",
			"video_id": videoID,
		})
	default:
		// If queue is full, unregister the job from activeVideoJobs as it won't be processed now.
		activeVideoJobsMutex.Lock()
		delete(activeVideoJobs, videoID) // Clean up: remove from active jobs as it won't be queued
		activeVideoJobsMutex.Unlock()
		log.Printf("Warning: HandleSummaryRequest: Job queue full for VideoID: %s, UserID: %s. Rejected job and removed from active jobs list.", videoID, userID)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":    "Server busy, job queue full. Please try again later.",
			"video_id": videoID,
		})
	}
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

// HandleSummaryEvents sets up an SSE connection for a client.
func HandleSummaryEvents(c *gin.Context) {
	// Authenticate user
	userInfo, authenticated := auth.GetSessionUser(c)
	if !authenticated || userInfo == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "인증된 사용자 정보를 찾을 수 없습니다."})
		return
	}
	userID := userInfo.ID

	// Set headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	// c.Writer.Header().Set("Access-Control-Allow-Origin", "*") // Consider security implications and set to specific frontend URL if possible

	// Create a channel for this client
	messageChan := make(chan []byte, 10) // Buffered channel (e.g., 10 messages)

	// Register client channel
	clientChannelsMutex.Lock()
	// If there's an existing channel for this user, close it before creating a new one.
	if oldChan, exists := clientChannels[userID]; exists {
		log.Printf("Info: HandleSummaryEvents: UserID %s reconnected to SSE. Closing previous channel.", userID)
		close(oldChan) // Close the old channel; its goroutine will terminate.
	}
	clientChannels[userID] = messageChan
	clientChannelsMutex.Unlock()
	log.Printf("Info: HandleSummaryEvents: SSE client connected: UserID %s. Channel registered.", userID)

	defer func() {
		clientChannelsMutex.Lock()
		// Only delete and close if the current channel in the map is the one this goroutine is managing.
		if currentChan, ok := clientChannels[userID]; ok && currentChan == messageChan {
			delete(clientChannels, userID)
			close(messageChan)
			log.Printf("Info: HandleSummaryEvents: SSE client disconnected: UserID %s. Channel deregistered and closed.", userID)
		} else {
			// This means the channel was already replaced by a newer connection or closed by another part of the code.
			log.Printf("Info: HandleSummaryEvents: SSE client disconnected: UserID %s. Channel was already replaced or deregistered, no action needed by this defer.", userID)
		}
		clientChannelsMutex.Unlock()
	}()
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		// This should ideally not happen with modern HTTP servers supporting http.Flusher
		log.Printf("Streaming unsupported for UserID %s!", userID)
		// Cannot send JSON error as headers might be partially written.
		// Just return to trigger the defer.
		return
	}

	// Send an initial connection confirmation event (optional, but good for client to know it's connected)
	// connectMsg := []byte("event: connected\ndata: {\"message\":\"SSE connection established\"}\n\n")
	// if _, err := c.Writer.Write(connectMsg); err != nil {
	// 	log.Printf("Error sending connection confirmation to UserID %s: %v", userID, err)
	// 	return // Trigger defer for cleanup
	// }
	// flusher.Flush()

	for {
		select {
		case message, open := <-messageChan:
			if !open { // True if messageChan was closed by the sender side
				log.Printf("Info: HandleSummaryEvents: SSE message channel for UserID %s closed by sender. Terminating stream.", userID)
				return
			}
			_, err := c.Writer.Write(message) // message should be pre-formatted SSE event string
			if err != nil {
				log.Printf("Warning: HandleSummaryEvents: Error writing to SSE client UserID %s: %v. Terminating stream.", userID, err)
				return // Error writing, client likely disconnected. Defer will clean up.
			}
			flusher.Flush()
		case <-c.Request.Context().Done(): // Client disconnected
			log.Printf("Info: HandleSummaryEvents: Client UserID %s context done (disconnected). Terminating SSE stream.", userID)
			return // Defer will clean up.
		}
	}
}
