package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/akirose/youtube-summarizer/auth"
	"github.com/akirose/youtube-summarizer/models"
	"github.com/akirose/youtube-summarizer/services"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// --- Mock Service Implementations ---

var (
	mockGetVideoInfo    func(videoID string) (*services.VideoInfo, error)
	originalGetVideoInfo func(videoID string) (*services.VideoInfo, error)

	mockGetTranscript    func(videoID string, chunkSize float64) ([][]services.TranscriptItem, error)
	originalGetTranscript func(videoID string, chunkSize float64) ([][]services.TranscriptItem, error)

	mockSummarizeChunks    func(chunks [][]services.TranscriptItem, userAPIKey string, userID string) (string, error)
	originalSummarizeChunks func(chunks [][]services.TranscriptItem, userAPIKey string, userID string) (string, error)
	
	// Mock for processSummarizationJob itself for some tests
	mockProcessSummarizationJob func(job SummarizationJob) (*SummaryResponse, error)
	originalProcessSummarizationJob func(job SummarizationJob) (*SummaryResponse, error)
)

func setupServiceMocks() {
	originalGetVideoInfo = services.GetVideoInfo
	services.GetVideoInfo = func(videoID string) (*services.VideoInfo, error) {
		if mockGetVideoInfo != nil {
			return mockGetVideoInfo(videoID)
		}
		return &services.VideoInfo{ID: videoID, Title: "Mock Video Title"}, nil
	}

	originalGetTranscript = services.GetTranscript
	services.GetTranscript = func(videoID string, chunkSize float64) ([][]services.TranscriptItem, error) {
		if mockGetTranscript != nil {
			return mockGetTranscript(videoID, chunkSize)
		}
		return [][]services.TranscriptItem{{{Text: "mock transcript", Start: 0, Duration: 5}}}, nil
	}

	originalSummarizeChunks = services.SummarizeChunks
	services.SummarizeChunks = func(chunks [][]services.TranscriptItem, userAPIKey string, userID string) (string, error) {
		if mockSummarizeChunks != nil {
			return mockSummarizeChunks(chunks, userAPIKey, userID)
		}
		return "mock summary", nil
	}
	
	// Keep original processSummarizationJob for direct testing, but allow mocking for handler tests
	originalProcessSummarizationJob = processSummarizationJob
	processSummarizationJob = func(job SummarizationJob) (*SummaryResponse, error) {
		if mockProcessSummarizationJob != nil {
			return mockProcessSummarizationJob(job)
		}
		// Fallback to original if no specific mock is set for this test
		return originalProcessSummarizationJob(job)
	}
}

func restoreServiceMocks() {
	services.GetVideoInfo = originalGetVideoInfo
	services.GetTranscript = originalGetTranscript
	services.SummarizeChunks = originalSummarizeChunks
	processSummarizationJob = originalProcessSummarizationJob
	
	// Reset mock function pointers
	mockGetVideoInfo = nil
	mockGetTranscript = nil
	mockSummarizeChunks = nil
	mockProcessSummarizationJob = nil
}

// --- Test Setup and Teardown ---

var testCacheDir string

func TestMain(m *testing.M) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create a temporary cache directory for tests
	var err error
	testCacheDir, err = os.MkdirTemp("", "summary_test_cache_")
	if err != nil {
		log.Fatalf("Failed to create temp cache dir: %v", err)
	}
	os.Setenv("CACHE_DIR", testCacheDir)
	
	setupServiceMocks()
	
	// Run tests
	exitCode := m.Run()
	
	restoreServiceMocks()
	// Clean up the temporary cache directory
	os.RemoveAll(testCacheDir)
	os.Unsetenv("CACHE_DIR")


	os.Exit(exitCode)
}

// resetGlobalState resets global variables used by the summary API for test isolation.
func resetGlobalStateAndInit(testQueueCapacity int) {
	// Reset job queue with specified capacity
	if testQueueCapacity <= 0 {
		testQueueCapacity = jobQueueCapacity // Use default if invalid
	}
	jobQueue = make(chan SummarizationJob, testQueueCapacity)

	// Reset SSE client channels
	clientChannelsMutex.Lock()
	for _, ch := range clientChannels {
		close(ch)
	}
	clientChannels = make(map[string]chan []byte)
	clientChannelsMutex.Unlock()

	// Reset active video jobs
	activeVideoJobsMutex.Lock()
	activeVideoJobs = make(map[string][]string)
	activeVideoJobsMutex.Unlock()
	
	// Re-initialize cache (or clear it)
	if summaryCache != nil {
		summaryCache.Clear() // Assuming Cache has a Clear method
	} else {
		// If InitCache wasn't called or failed, try to init it.
		// This might happen if tests run InitSummaryModule selectively.
		if err := InitCache(); err != nil {
			log.Printf("Warning: Failed to re-initialize cache in resetGlobalStateAndInit: %v", err)
		}
	}


	// Note: InitSummaryModule also starts the worker pool. For some tests,
	// we might want to control worker startup manually or mock processSummarizationJob.
	// For now, InitSummaryModule will be called by tests needing the full module setup.
}


// --- Helper Functions ---

func createTestRouter() *gin.Engine {
	r := gin.Default()
	// Setup session middleware (required by auth.GetSessionUser)
	store := cookie.NewStore([]byte("secret")) // Use a consistent secret for testing
	r.Use(sessions.Sessions("mysession", store))
	return r
}

func createTestContext(r *gin.Engine, method, path string, body io.Reader) (*gin.Context, *httptest.ResponseRecorder) {
	req, _ := http.NewRequest(method, path, body)
	if method == "POST" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	// For auth.GetSessionUser to work, it needs a session.
	// We ensure the session middleware is used in createTestRouter.
	// Then, individual tests can populate the session.
	return c, w
}

func mockAuthUser(c *gin.Context, userID, userName, email string) {
	session := sessions.Default(c)
	session.Set("user", &auth.User{
		ID:    userID,
		Name:  userName,
		Email: email,
		// Picture might not be needed for summary tests
	})
	session.Set("authenticated", true)
	if err := session.Save(); err != nil {
		log.Printf("Error saving session in mockAuthUser: %v", err)
	}
}


// --- Test Cases ---
// Placeholder for now

func TestPlaceholder(t *testing.T) {
	assert.True(t, true, "This is a placeholder test.")
}

// TODO: Add detailed test cases as per requirements
// TestEnqueueJob
// TestWorkerProcessing
// TestQueueFull
// TestSSESuccessNotification
// TestSSEErrorNotification
// TestSSEClientDisconnect
// TestDeduplicationLogic
// TestDeduplicationWithCache

// TestInitSummaryModule to ensure it runs without error and starts workers
func TestInitSummaryModule_StartsWorkers(t *testing.T) {
	// This test primarily checks if InitSummaryModule can be called without panic
	// and that it attempts to start workers.
	// Worker behavior itself will be tested more specifically.

	// Reset globals and re-initialize the module
	resetGlobalStateAndInit(jobQueueCapacity) // Use default capacity
	
	// Temporarily redirect log output to check for worker start messages
	var logBuf bytes.Buffer
	originalLogger := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(originalLogger)

	err := InitSummaryModule()
	assert.NoError(t, err, "InitSummaryModule should not return an error")
	
	// Check log output for worker start messages
	// This is a bit of a white-box test, but confirms workers are being started.
	// Give a small delay for goroutines to start and log.
	time.Sleep(10 * time.Millisecond)
	logs := logBuf.String()
	assert.Contains(t, logs, "Info: Summarization worker pool configured with", "Log should indicate worker pool configuration")
	assert.Contains(t, logs, "Info: Worker 0 starting.", "Log should indicate worker 0 starting") // Assuming at least one worker
	
	// Clean up jobQueue to stop workers if they were started by InitSummaryModule
	// This prevents them from interfering with subsequent tests that might not expect active workers.
	// Since jobQueue is global, closing it here.
	// However, a better approach for tests that don't want workers running is to mock startWorkerPool
	// or ensure jobQueue is empty and no jobs are pushed.
	// For now, we'll close it. This means InitSummaryModule should not be called again in the same test run
	// without re-making the jobQueue.
	// This highlights a challenge with global state in tests.
	if jobQueue != nil {
		close(jobQueue) 
		// Allow time for workers to shut down
		time.Sleep(50 * time.Millisecond) 
	}
}

// TestHandleSummaryRequest_Cached tests fetching a cached summary.
func TestHandleSummaryRequest_Cached(t *testing.T) {
	resetGlobalStateAndInit(jobQueueCapacity)
	_ = InitSummaryModule() // Initialize cache, etc. but we'll close jobQueue to stop workers for this test.
	if jobQueue != nil { close(jobQueue); time.Sleep(10 * time.Millisecond); }


	router := createTestRouter()
	router.POST("/api/summary", HandleSummaryRequest)

	// Prepare a cached item
	videoID := "cachedVideo1"
	cachedTitle := "Cached Video Title"
	cachedSummaryText := "This is a cached summary."
	
	err := summaryCache.Set(videoID, cachedTitle, cachedSummaryText, nil, []services.TranscriptItem{})
	assert.NoError(t, err)
	

	// Create request
	reqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)}
	jsonBody, _ := json.Marshal(reqBody)
	c, w := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBody))
	mockAuthUser(c, "user123", "Test User", "test@example.com")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp SummaryResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Cached)
	assert.Equal(t, videoID, resp.VideoID)
	assert.Equal(t, cachedTitle, resp.Title)
	assert.Equal(t, cachedSummaryText, resp.Summary)
}

// TestHandleSummaryRequest_EnqueueJob tests that a job is enqueued for a non-cached video.
func TestHandleSummaryRequest_EnqueueJob(t *testing.T) {
	resetGlobalStateAndInit(1) // Queue capacity of 1
	// Manually initialize cache as InitSummaryModule (which starts workers) is not called here.
	if err := InitCache(); err != nil {
		t.Fatalf("Failed to init cache: %v", err)
	}

	router := createTestRouter()
	router.POST("/api/summary", HandleSummaryRequest)

	videoID := "enqueueVideo1"
	userID := "userEnqueue"
	reqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)}
	jsonBody, _ := json.Marshal(reqBody)

	c, w := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBody))
	mockAuthUser(c, userID, "Enqueue User", "enqueue@example.com")

	// Mock GetVideoInfo as it's called before queuing
	mockGetVideoInfo = func(vid string) (*services.VideoInfo, error) {
		assert.Equal(t, videoID, vid)
		return &services.VideoInfo{ID: vid, Title: "Enqueue Test Video"}, nil
	}
	defer func() { mockGetVideoInfo = nil }() // Restore

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusAccepted, w.Code, "Response code should be 202 Accepted for enqueued job")

	// Verify job is in the queue
	select {
	case job := <-jobQueue:
		assert.Equal(t, videoID, job.VideoID, "VideoID in job should match")
		assert.Equal(t, userID, job.UserID, "UserID in job should match")
	case <-time.After(100 * time.Millisecond): // Timeout to prevent test hanging
		t.Fatal("Job was not enqueued within timeout")
	}

	// Ensure activeVideoJobs has the entry
	activeVideoJobsMutex.RLock()
	subscribers, ok := activeVideoJobs[videoID]
	activeVideoJobsMutex.RUnlock()
	assert.True(t, ok, "VideoID should be in activeVideoJobs")
	assert.Contains(t, subscribers, userID, "UserID should be a subscriber")
}

// TestWorkerProcessing tests worker picking up a job and processing it.
func TestWorkerProcessing(t *testing.T) {
	resetGlobalStateAndInit(5) // Queue capacity of 5
	// Call InitSummaryModule to start the workers.
	// We will mock processSummarizationJob to control its behavior.
	err := InitSummaryModule()
	assert.NoError(t, err)
	defer func() {
		if jobQueue != nil {
			close(jobQueue) // Stop workers
			time.Sleep(50 * time.Millisecond) // Allow workers to shut down
		}
	}()


	videoID := "workerVideo1"
	userID := "workerUser1"
	apiKey := "testKey"

	// --- Sub-test for successful processing ---
	t.Run("SuccessfulProcessing", func(t *testing.T) {
		processedJobChan := make(chan SummarizationJob, 1)
		expectedSummary := &SummaryResponse{VideoID: videoID, Title: "Processed Title", Summary: "Processed Summary"}

		// Mock processSummarizationJob for success
		mockProcessSummarizationJob = func(job SummarizationJob) (*SummaryResponse, error) {
			processedJobChan <- job
			return expectedSummary, nil
		}
		defer func() { mockProcessSummarizationJob = nil }() // Restore for other sub-tests or tests

		// Manually add to activeVideoJobs as HandleSummaryRequest would
		activeVideoJobsMutex.Lock()
		activeVideoJobs[videoID] = []string{userID}
		activeVideoJobsMutex.Unlock()
		
		// Enqueue a job directly
		jobQueue <- SummarizationJob{VideoID: videoID, UserID: userID, APIKey: apiKey, URL: "testURL"}

		// Wait for the job to be processed
		select {
		case processed := <-processedJobChan:
			assert.Equal(t, videoID, processed.VideoID)
			assert.Equal(t, userID, processed.UserID)
		case <-time.After(500 * time.Millisecond): // Increased timeout for CI/slower environments
			t.Fatal("Timeout waiting for worker to process job")
		}
		
		// Check activeVideoJobs is cleared
		activeVideoJobsMutex.RLock()
		_, stillActive := activeVideoJobs[videoID]
		activeVideoJobsMutex.RUnlock()
		assert.False(t, stillActive, "Job should be removed from activeVideoJobs after processing")
	})

	// --- Sub-test for error processing ---
	t.Run("ErrorProcessing", func(t *testing.T) {
		processedJobChan := make(chan SummarizationJob, 1)
		expectedError := errors.New("summarization failed")

		// Mock processSummarizationJob for error
		mockProcessSummarizationJob = func(job SummarizationJob) (*SummaryResponse, error) {
			processedJobChan <- job
			return nil, expectedError
		}
		defer func() { mockProcessSummarizationJob = nil }()

		// Manually add to activeVideoJobs
		videoIDErr := "workerVideoErr"
		userIDErr := "workerUserErr"
		activeVideoJobsMutex.Lock()
		activeVideoJobs[videoIDErr] = []string{userIDErr}
		activeVideoJobsMutex.Unlock()

		jobQueue <- SummarizationJob{VideoID: videoIDErr, UserID: userIDErr, APIKey: apiKey, URL: "testURLErr"}

		select {
		case processed := <-processedJobChan:
			assert.Equal(t, videoIDErr, processed.VideoID)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for worker to process job (error case)")
		}
		
		// Check activeVideoJobs is cleared
		activeVideoJobsMutex.RLock()
		_, stillActive := activeVideoJobs[videoIDErr]
		activeVideoJobsMutex.RUnlock()
		assert.False(t, stillActive, "Job should be removed from activeVideoJobs after error processing")
	})
}

// TestHandleSummaryRequest_QueueFull tests the behavior when the job queue is full.
func TestHandleSummaryRequest_QueueFull(t *testing.T) {
	queueCapacity := 1
	resetGlobalStateAndInit(queueCapacity)
	// Manually initialize cache as InitSummaryModule (which starts workers) is not called
	// to prevent workers from consuming the job we put in the queue.
	if err := InitCache(); err != nil {
		t.Fatalf("Failed to init cache: %v", err)
	}

	router := createTestRouter()
	router.POST("/api/summary", HandleSummaryRequest)

	// --- Fill the queue to its capacity ---
	// Mock GetVideoInfo for the first job that fills the queue
	originalVideoID := "originalVideoID"
	mockGetVideoInfo = func(vid string) (*services.VideoInfo, error) {
		if vid == originalVideoID {
			return &services.VideoInfo{ID: vid, Title: "Original Video"}, nil
		}
		return &services.VideoInfo{ID: vid, Title: "Another Video"}, nil // For the second request
	}
	defer func() { mockGetVideoInfo = nil }()

	// Simulate the first request that successfully queues a job
	firstUserID := "userFirst"
	firstReqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", originalVideoID)}
	firstJsonBody, _ := json.Marshal(firstReqBody)
	firstC, firstW := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(firstJsonBody))
	mockAuthUser(firstC, firstUserID, "First User", "first@example.com")
	router.ServeHTTP(firstW, firstC.Request)
	assert.Equal(t, http.StatusAccepted, firstW.Code, "First job should be accepted")

	// Verify the first job is in the queue
	select {
	case job := <-jobQueue:
		assert.Equal(t, originalVideoID, job.VideoID)
		// Put it back so the queue is full for the next request
		jobQueue <- job
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("First job was not enqueued as expected")
	}
	// At this point, jobQueue should be full (contains one item, capacity is 1)

	// --- Attempt to enqueue another job when queue is full ---
	secondVideoID := "fullQueueVideoID"
	secondUserID := "userQueueFull"
	secondReqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", secondVideoID)}
	secondJsonBody, _ := json.Marshal(secondReqBody)

	secondC, secondW := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(secondJsonBody))
	mockAuthUser(secondC, secondUserID, "Second User", "second@example.com")

	router.ServeHTTP(secondW, secondC.Request)

	assert.Equal(t, http.StatusServiceUnavailable, secondW.Code, "Response code should be 503 Service Unavailable for full queue")

	// Verify that the second job's videoID is NOT in activeVideoJobs (or was removed)
	activeVideoJobsMutex.RLock()
	_, isActive := activeVideoJobs[secondVideoID]
	activeVideoJobsMutex.RUnlock()
	assert.False(t, isActive, "Job attempted with full queue should not be in activeVideoJobs or should have been removed")

	// Clean up: empty the queue
	select {
	case <-jobQueue: // remove the one job
	default:
	}
}

// TestHandleSummaryEvents_Connection tests the SSE connection and disconnection.
func TestHandleSummaryEvents_Connection(t *testing.T) {
	resetGlobalStateAndInit(jobQueueCapacity) // Use default capacity, though queue not directly used here
	err := InitSummaryModule()                // Initializes clientChannels
	assert.NoError(t, err)
	defer func() {
		if jobQueue != nil {
			close(jobQueue)
			time.Sleep(50 * time.Millisecond)
		}
	}()


	router := createTestRouter()
	router.GET("/api/summary/events", HandleSummaryEvents)

	userID := "sseUser1"

	// Create a context that can be canceled to simulate client disconnect
	req, _ := http.NewRequest("GET", "/api/summary/events", nil)
	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background()) // Create a cancellable context
	req = req.WithContext(ctx)

	c, _ := gin.CreateTestContext(w)
	c.Request = req
	mockAuthUser(c, userID, "SSE User", "sse@example.com")
	
	// Run HandleSummaryEvents in a goroutine as it's a blocking call
	go func() {
		router.ServeHTTP(w, c.Request)
	}()

	// Wait a bit for the server to register the client
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, http.StatusOK, w.Code, "SSE endpoint should return 200 OK")
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	clientChannelsMutex.RLock()
	_, exists := clientChannels[userID]
	clientChannelsMutex.RUnlock()
	assert.True(t, exists, "Client channel should be registered")

	// Simulate client disconnect by canceling the context
	cancel()

	// Wait a bit for the server to process disconnect and clean up
	time.Sleep(100 * time.Millisecond) // Increased wait time

	clientChannelsMutex.RLock()
	_, stillExists := clientChannels[userID]
	clientChannelsMutex.RUnlock()
	assert.False(t, stillExists, "Client channel should be removed after disconnect")
}

// TestSSENotifications tests the end-to-end flow of SSE notifications.
func TestSSENotifications(t *testing.T) {
	resetGlobalStateAndInit(5) // Queue with capacity 5
	err := InitSummaryModule()
	assert.NoError(t, err, "InitSummaryModule should not return an error")
	defer func() {
		if jobQueue != nil {
			close(jobQueue) // Stop workers
			time.Sleep(100 * time.Millisecond) // Allow workers to shut down
		}
		// Ensure all client channels are closed and cleared after tests
		clientChannelsMutex.Lock()
		for uid, ch := range clientChannels {
			close(ch)
			delete(clientChannels, uid)
		}
		clientChannelsMutex.Unlock()
	}()

	router := createTestRouter()
	router.POST("/api/summary", HandleSummaryRequest)
	router.GET("/api/summary/events", HandleSummaryEvents)

	userID := "sseNotifyUser1"
	videoID := "sseNotifyVideo1"

	// --- Setup SSE Client ---
	sseReq, _ := http.NewRequest("GET", "/api/summary/events", nil)
	sseWriter := httptest.NewRecorder() // This recorder will capture SSE events
	sseCtx, sseCancel := context.WithCancel(context.Background())
	sseReq = sseReq.WithContext(sseCtx)
	defer sseCancel() // Ensure context is cancelled after test

	sseGinCtx, _ := gin.CreateTestContext(sseWriter)
	sseGinCtx.Request = sseReq
	mockAuthUser(sseGinCtx, userID, "SSE Notify User", "ssenotify@example.com")
	
	// Run SSE handler in a goroutine
	var sseWg sync.WaitGroup
	sseWg.Add(1)
	go func() {
		defer sseWg.Done()
		router.ServeHTTP(sseWriter, sseGinCtx.Request)
	}()
	time.Sleep(50 * time.Millisecond) // Allow SSE connection to establish

	// Mock GetVideoInfo as it's called by HandleSummaryRequest before queuing
	mockGetVideoInfo = func(vid string) (*services.VideoInfo, error) {
		return &services.VideoInfo{ID: vid, Title: "SSE Test Video"}, nil
	}
	defer func() { mockGetVideoInfo = nil }()


	// --- Sub-test for successful SSE notification ---
	t.Run("SuccessNotification", func(t *testing.T) {
		expectedSummary := &SummaryResponse{VideoID: videoID, Title: "SSE Success Title", Summary: "SSE Success Summary", Cached: false}
		
		// Mock processSummarizationJob for success
		mockProcessSummarizationJob = func(job SummarizationJob) (*SummaryResponse, error) {
			// Simulate processing time
			time.Sleep(100 * time.Millisecond)
			return expectedSummary, nil
		}
		defer func() { mockProcessSummarizationJob = nil }() // Restore

		// Make POST request to trigger summary
		reqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)}
		jsonBody, _ := json.Marshal(reqBody)
		postC, postW := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBody))
		mockAuthUser(postC, userID, "SSE Notify User", "ssenotify@example.com") // Same user as SSE connection
		
		router.ServeHTTP(postW, postC.Request)
		assert.Equal(t, http.StatusAccepted, postW.Code)

		// Wait for SSE event or timeout
		// Reading from ResponseRecorder.Body is tricky for streaming. We check the flushed output.
		// A better way might be to use a custom ResponseWriter that captures flushed chunks.
		// For now, we'll rely on a timeout and then check the body.
		// This part is inherently a bit flaky with httptest.ResponseRecorder for true streaming.
		
		var receivedEvent bool
		var eventData string
		expectedEventPrefix := fmt.Sprintf("event: summary_complete\ndata: ")
		
		// Poll for the event with a timeout
		timeout := time.After(1 * time.Second) // Increased timeout
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

	LoopSuccess:
		for {
			select {
			case <-ticker.C:
				bodyStr := sseWriter.Body.String()
				if strings.Contains(bodyStr, expectedEventPrefix) {
					lines := strings.Split(strings.TrimSpace(bodyStr), "\n\n")
					for _, lineSet := range lines {
						if strings.HasPrefix(lineSet, expectedEventPrefix) {
							eventData = strings.TrimPrefix(lineSet, expectedEventPrefix)
							receivedEvent = true
							break LoopSuccess
						}
					}
				}
			case <-timeout:
				t.Logf("SSE Success - Recorder Body: %s", sseWriter.Body.String())
				t.Fatal("Timeout waiting for summary_complete SSE event")
			}
		}

		assert.True(t, receivedEvent, "Should receive summary_complete event")
		var receivedSummary SummaryResponse
		err := json.Unmarshal([]byte(eventData), &receivedSummary)
		assert.NoError(t, err, "Failed to unmarshal SSE summary_complete data")
		assert.Equal(t, expectedSummary.VideoID, receivedSummary.VideoID)
		assert.Equal(t, expectedSummary.Title, receivedSummary.Title)
		assert.Equal(t, expectedSummary.Summary, receivedSummary.Summary)
		
		// Clean up active job for the next sub-test
		activeVideoJobsMutex.Lock()
		delete(activeVideoJobs, videoID)
		activeVideoJobsMutex.Unlock()
	})
	
	// --- Sub-test for error SSE notification ---
	videoIDErr := "sseNotifyVideoErr" // Use a different video ID for this sub-test
	t.Run("ErrorNotification", func(t *testing.T) {
		expectedErrorMsg := "SSE processing failed"
		
		mockProcessSummarizationJob = func(job SummarizationJob) (*SummaryResponse, error) {
			time.Sleep(100 * time.Millisecond)
			return nil, errors.New(expectedErrorMsg)
		}
		defer func() { mockProcessSummarizationJob = nil }()

		reqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoIDErr)}
		jsonBody, _ := json.Marshal(reqBody)
		postC, postW := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBody))
		// Important: Use the same userID that established the SSE connection
		mockAuthUser(postC, userID, "SSE Notify User", "ssenotify@example.com")
		
		router.ServeHTTP(postW, postC.Request)
		assert.Equal(t, http.StatusAccepted, postW.Code)

		var receivedEvent bool
		var eventDataStr string
		expectedEventPrefix := fmt.Sprintf("event: summary_error\ndata: ")

		timeout := time.After(1 * time.Second)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

	LoopError:
		for {
			select {
			case <-ticker.C:
				bodyStr := sseWriter.Body.String() // sseWriter is from the parent scope, capturing all events
				if strings.Contains(bodyStr, expectedEventPrefix) {
					// Find the last occurrence of summary_error for this videoID, in case previous events exist
					lines := strings.Split(strings.TrimSpace(bodyStr), "\n\n")
					for i := len(lines) - 1; i >= 0; i-- {
						lineSet := lines[i]
						if strings.HasPrefix(lineSet, expectedEventPrefix) {
							tempData := strings.TrimPrefix(lineSet, expectedEventPrefix)
							var parsedData map[string]string
							if json.Unmarshal([]byte(tempData), &parsedData) == nil {
								if parsedData["videoId"] == videoIDErr {
									eventDataStr = tempData
									receivedEvent = true
									break LoopError
								}
							}
						}
					}
				}
			case <-timeout:
				t.Logf("SSE Error - Recorder Body: %s", sseWriter.Body.String())
				t.Fatal("Timeout waiting for summary_error SSE event")
			}
		}
		
		assert.True(t, receivedEvent, "Should receive summary_error event")
		var receivedError map[string]string
		err := json.Unmarshal([]byte(eventDataStr), &receivedError)
		assert.NoError(t, err, "Failed to unmarshal SSE summary_error data")
		assert.Equal(t, videoIDErr, receivedError["videoId"])
		assert.Equal(t, expectedErrorMsg, receivedError["error"])

		// Clean up active job
		activeVideoJobsMutex.Lock()
		delete(activeVideoJobs, videoIDErr)
		activeVideoJobsMutex.Unlock()
	})

	// Cancel context for SSE handler and wait for it to stop
	sseCancel()
	sseWg.Wait()
}

// TestDeduplication_MultipleSubscribers tests job deduplication and notification to all subscribers.
func TestDeduplication_MultipleSubscribers(t *testing.T) {
	resetGlobalStateAndInit(5)
	err := InitSummaryModule()
	assert.NoError(t, err)
	defer func() {
		if jobQueue != nil { close(jobQueue); time.Sleep(100 * time.Millisecond); }
		clientChannelsMutex.Lock()
		for uid, ch := range clientChannels { close(ch); delete(clientChannels, uid); }
		clientChannelsMutex.Unlock()
	}()

	router := createTestRouter()
	router.POST("/api/summary", HandleSummaryRequest)
	router.GET("/api/summary/events", HandleSummaryEvents)

	videoID := "dedupVideo1"
	userA := "userA_dedup"
	userB := "userB_dedup"

	// --- Setup SSE Client for User A ---
	sseWriterA := httptest.NewRecorder()
	sseCtxA, sseCancelA := context.WithCancel(context.Background())
	defer sseCancelA()
	reqA, _ := http.NewRequest("GET", "/api/summary/events", nil)
	reqA = reqA.WithContext(sseCtxA)
	sseGinCtxA, _ := gin.CreateTestContext(sseWriterA); sseGinCtxA.Request = reqA
	mockAuthUser(sseGinCtxA, userA, "User A", "a@example.com")
	var sseWgA sync.WaitGroup; sseWgA.Add(1)
	go func() { defer sseWgA.Done(); router.ServeHTTP(sseWriterA, sseGinCtxA.Request) }()
	time.Sleep(50 * time.Millisecond)

	// --- Setup SSE Client for User B ---
	sseWriterB := httptest.NewRecorder()
	sseCtxB, sseCancelB := context.WithCancel(context.Background())
	defer sseCancelB()
	reqB, _ := http.NewRequest("GET", "/api/summary/events", nil)
	reqB = reqB.WithContext(sseCtxB)
	sseGinCtxB, _ := gin.CreateTestContext(sseWriterB); sseGinCtxB.Request = reqB
	mockAuthUser(sseGinCtxB, userB, "User B", "b@example.com")
	var sseWgB sync.WaitGroup; sseWgB.Add(1)
	go func() { defer sseWgB.Done(); router.ServeHTTP(sseWriterB, sseGinCtxB.Request) }()
	time.Sleep(50 * time.Millisecond)
	
	// Mock GetVideoInfo
	mockGetVideoInfo = func(vid string) (*services.VideoInfo, error) {
		return &services.VideoInfo{ID: vid, Title: "Dedup Test Video"}, nil
	}
	defer func() { mockGetVideoInfo = nil }()

	// Mock processSummarizationJob
	expectedSummary := &SummaryResponse{VideoID: videoID, Title: "Dedup Success Title", Summary: "Dedup Success Summary"}
	jobProcessedSignal := make(chan bool, 1)
	mockProcessSummarizationJob = func(job SummarizationJob) (*SummaryResponse, error) {
		time.Sleep(100 * time.Millisecond) // Simulate work
		jobProcessedSignal <- true
		return expectedSummary, nil
	}
	defer func() { mockProcessSummarizationJob = nil }()

	// --- User A requests video V1 ---
	reqBodyA := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)}
	jsonBodyA, _ := json.Marshal(reqBodyA)
	postCA, postWA := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBodyA))
	mockAuthUser(postCA, userA, "User A", "a@example.com")
	router.ServeHTTP(postWA, postCA.Request)
	assert.Equal(t, http.StatusAccepted, postWA.Code, "User A's request should be accepted")

	// Verify User A is a subscriber
	activeVideoJobsMutex.RLock()
	subsA, okA := activeVideoJobs[videoID]
	activeVideoJobsMutex.RUnlock()
	assert.True(t, okA, "VideoID should be in activeVideoJobs after User A's request")
	assert.Contains(t, subsA, userA, "User A should be a subscriber")
	assert.Equal(t, 1, len(jobQueue), "One job should be in queue")


	// --- User B requests video V1 ---
	reqBodyB := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)}
	jsonBodyB, _ := json.Marshal(reqBodyB)
	postCB, postWB := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBodyB))
	mockAuthUser(postCB, userB, "User B", "b@example.com")
	router.ServeHTTP(postWB, postCB.Request)
	assert.Equal(t, http.StatusAccepted, postWB.Code, "User B's request should also be accepted (job already active)")
	
	// Verify User B is now also a subscriber, and still only one job in queue
	activeVideoJobsMutex.RLock()
	subsB, okB := activeVideoJobs[videoID]
	activeVideoJobsMutex.RUnlock()
	assert.True(t, okB, "VideoID should still be in activeVideoJobs")
	assert.Contains(t, subsB, userA, "User A should still be a subscriber")
	assert.Contains(t, subsB, userB, "User B should now be a subscriber")
	assert.Equal(t, 1, len(jobQueue), "Still only one job should be in queue")


	// Wait for job to be processed by worker
	select {
	case <-jobProcessedSignal:
		// continue
	case <-time.After(2 * time.Second): // Increased timeout
		t.Fatal("Timeout waiting for job to be processed by worker")
	}

	// --- Verify SSE notifications for both users ---
	expectedEventData, _ := json.Marshal(expectedSummary)
	expectedSSEEvent := fmt.Sprintf("event: summary_complete\ndata: %s\n\n", string(expectedEventData))

	// Helper to check SSE response
	checkSSEResponse := func(t *testing.T, user string, writer *httptest.ResponseRecorder, expectedEvent string) {
		var receivedEvent bool
		timeout := time.After(1 * time.Second)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
	Loop:
		for {
			select {
			case <-ticker.C:
				if strings.Contains(writer.Body.String(), expectedEvent) {
					receivedEvent = true
					break Loop
				}
			case <-timeout:
				t.Logf("SSE for %s - Recorder Body: %s", user, writer.Body.String())
				t.Fatalf("Timeout waiting for SSE event for %s", user)
			}
		}
		assert.True(t, receivedEvent, fmt.Sprintf("User %s should receive summary_complete event", user))
		assert.Contains(t, writer.Body.String(), expectedEvent, fmt.Sprintf("SSE event for %s is incorrect", user))
	}

	checkSSEResponse(t, userA, sseWriterA, expectedSSEEvent)
	checkSSEResponse(t, userB, sseWriterB, expectedSSEEvent)

	// Verify active job is cleared
	activeVideoJobsMutex.RLock()
	_, stillActive := activeVideoJobs[videoID]
	activeVideoJobsMutex.RUnlock()
	assert.False(t, stillActive, "Job should be cleared from activeVideoJobs after processing and notification")

	sseCancelA()
	sseCancelB()
	sseWgA.Wait()
	sseWgB.Wait()
}

func TestDeduplication_DoesNotAffectCached(t *testing.T) {
	resetGlobalStateAndInit(5)
	err := InitSummaryModule() // Initializes cache
	assert.NoError(t, err)
	defer func() { if jobQueue != nil { close(jobQueue); time.Sleep(50 * time.Millisecond); } }()

	router := createTestRouter()
	router.POST("/api/summary", HandleSummaryRequest)

	videoID := "cachedDedupVideo"
	userID := "userCachedDedup"

	// Pre-cache a summary
	cachedTitle := "Cached Dedup Title"
	cachedSummaryText := "This is a cached summary for deduplication test."
	err = summaryCache.Set(videoID, cachedTitle, cachedSummaryText, nil, []services.TranscriptItem{})
	assert.NoError(t, err)

	// User C requests Video V2 (cached)
	reqBody := SummaryRequest{URL: fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)}
	jsonBody, _ := json.Marshal(reqBody)
	postC, postW := createTestContext(router, "POST", "/api/summary", bytes.NewBuffer(jsonBody))
	mockAuthUser(postC, userID, "User C", "c@example.com")

	router.ServeHTTP(postW, postC.Request)

	assert.Equal(t, http.StatusOK, postW.Code, "Response should be 200 OK for cached video")
	var resp SummaryResponse
	err = json.Unmarshal(postW.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp.Cached, "Response should indicate cached=true")
	assert.Equal(t, videoID, resp.VideoID)

	// Verify no job was queued
	assert.Equal(t, 0, len(jobQueue), "No job should be queued for a cached video")

	// Verify activeVideoJobs is not populated for this videoID
	activeVideoJobsMutex.RLock()
	_, isActive := activeVideoJobs[videoID]
	activeVideoJobsMutex.RUnlock()
	assert.False(t, isActive, "Cached video should not create an entry in activeVideoJobs")
}
