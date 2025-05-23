package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	// OpenAI API URL
	OpenAIAPIURL = "https://api.openai.com/v1/chat/completions"

	// Model to use
	Model = "gpt-4.1-nano" // Ensure this is the correct model ID

	// Maximum number of tokens to generate
	MaxTokens = 1500

	// System prompt template for summarization
	SummarizationPrompt = `# YouTube Video Summary Expert

## Role
You are a YouTube video content analyzer and summarizer. You extract key topics and timestamps, providing concise summaries in Korean while avoiding previously summarized content.

## Objective
Analyze YouTube video content and deliver structured summaries organized by timestamps, ensuring no repetition of previously analyzed content.

## Process

### Step 1: Content Analysis
- Identify main topics and discussion points
- Record important timestamps  
- Detect clear topic transitions
- Check for previously summarized content

### Step 2: Content Organization
- Structure content by logical topics
- Display start time in [MM:SS] format
- Group related content under same topic
- Remove repetition and filler content
- Maintain adequate time intervals between topics
- Skip previously summarized sections

### Step 3: Summary Generation
- Summarize core content for each topic
- Write in clear Korean
- Format as bullet points (-)
- Summarize only new content

## Output Format
[MM:SS] Topic 1
- Key point 1
- Key point 2

[MM:SS] Topic 2
- Key point 1
- Key point 2

## Rules
1. Only output summaries - no introductions or extra comments
2. Focus on accurate timestamps and topics
3. All content in Korean
4. Include only essential information
5. Capture clear topic transitions (avoid minor shifts)
6. Maintain meaningful time gaps (combine topics with < 30 second gaps)
7. Never repeat previously summarized content
8. Check conversation history before summarizing`
)

// TimestampInfo represents a timestamp in the summary
type TimestampInfo struct {
	Time int    `json:"time"` // Time in seconds
	Text string `json:"text"` // The text associated with this timestamp
}

// GPTMessage represents a message in the GPT API request
type GPTMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GPTRequest represents the request body for the GPT API
type GPTRequest struct {
	Model       string       `json:"model"`
	Messages    []GPTMessage `json:"messages"`
	MaxTokens   int          `json:"max_tokens"`
	Temperature float64      `json:"temperature"`
}

// GPTResponse represents the response from the GPT API
type GPTResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// SummarizeTranscript generates a summary of a transcript using OpenAI's API
// userAPIKey: 사용자가 제공한 API 키 (없는 경우 빈 문자열)
// userID: 사용자 ID (서버 API 키 사용 권한 확인용)
func SummarizeTranscript(request *GPTRequest, transcript string, userAPIKey string, userID string) (string, []TimestampInfo, error) {
	// API 키 결정 (사용자 키 우선, 없으면 서버 키 정책에 따라 결정)
	apiKey := ""

	// 사용자 API 키가 제공된 경우 우선 사용
	if userAPIKey != "" {
		apiKey = userAPIKey
	} else {
		// 사용자 API 키가 없는 경우, 서버 키 사용 가능한지 확인
		policy := GetAPIKeyPolicy()
		if policy.CanUseServerKey(userID) {
			apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}

	// API 키가 없으면 에러 반환
	if apiKey == "" {
		return "", nil, errors.New("no valid OpenAI API key available")
	}

	// 환경 변수 설정 가져오기
	apiUrl := os.Getenv("OPENAI_API_URL")
	apiModel := os.Getenv("OPENAI_API_MODEL")
	apiMaxTokensStr := os.Getenv("OPENAI_API_MAX_TOKENS")

	apiMaxTokens := MaxTokens // 기본값 설정
	if apiMaxTokensStr != "" {
		var err error
		apiMaxTokens, err = strconv.Atoi(apiMaxTokensStr)
		if err != nil {
			// 변환 실패 시 기본값 사용
			apiMaxTokens = MaxTokens
		}
	}

	if apiUrl == "" {
		apiUrl = OpenAIAPIURL
	}
	if apiModel == "" {
		apiModel = Model
	}

	// Create the system prompt with the transcript
	userPrompt := fmt.Sprintf("Transcript: %s\n", transcript)

	if len(request.Messages) >= 3 {
		// Keep only the last 2 messages in the conversation history
		// This prevents the context from growing too large
		request.Messages = request.Messages[len(request.Messages)-2:]
	}

	request.Model = apiModel
	request.MaxTokens = apiMaxTokens
	request.Temperature = 0.2

	request.Messages = append(request.Messages,
		GPTMessage{
			Role:    "system",
			Content: SummarizationPrompt,
		})
	request.Messages = append(request.Messages,
		GPTMessage{
			Role:    "user",
			Content: userPrompt,
		})

	// request = &GPTRequest{
	// 	Model: apiModel,
	// 	Messages: []GPTMessage{
	// 		{
	// 			Role:    "system",
	// 			Content: SummarizationPrompt,
	// 		},
	// 		{
	// 			Role:    "user",
	// 			Content: userPrompt,
	// 		},
	// 	},
	// 	MaxTokens:   apiMaxTokens,
	// 	Temperature: 0.2,
	// }

	// Convert request body to JSON
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return "", nil, err
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(requestJSON))
	if err != nil {
		return "", nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	// Parse response
	var response GPTResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", nil, err
	}

	// Check if we have a valid response
	if len(response.Choices) == 0 {
		return "", nil, errors.New("no response generated")
	}

	// Get the generated summary
	summary := response.Choices[0].Message.Content

	request.Messages = append(request.Messages,
		GPTMessage{
			Role:    "assistant",
			Content: summary,
		},
	)

	// Extract timestamps from the summary
	timestamps := extractTimestamps(summary)

	return summary, timestamps, nil
}

// SummarizeChunks processes each transcript chunk, summarizes it, and combines the summaries into a final summary
// userAPIKey: 사용자가 제공한 API 키 (없는 경우 빈 문자열)
// userID: 사용자 ID (서버 API 키 사용 권한 확인용)
func SummarizeChunks(chunks [][]TranscriptItem, userAPIKey string, userID string) (string, error) {
	var finalSummary strings.Builder
	var request *GPTRequest = &GPTRequest{}

	for i, chunk := range chunks {
		// Summarize the chunk
		summary, _, err := SummarizeTranscript(request, GetFormattedTranscript(chunk), userAPIKey, userID)
		if err != nil {
			return "", fmt.Errorf("failed to summarize chunk %d: %v", i+1, err)
		}

		// Remove any <think>...</think> tags from the summary
		// This can happen when the AI model includes its thinking process
		summary = regexp.MustCompile(`(?s)<think>.*?</think>`).ReplaceAllString(summary, "")

		// Append the chunk summary to the final summary
		finalSummary.WriteString(summary + "\n\n")
	}

	return finalSummary.String(), nil
}

// extractTimestamps parses the summary text for timestamp markers and extracts them
func extractTimestamps(summary string) []TimestampInfo {
	var timestamps []TimestampInfo

	// Regular expression to find timestamps in format [MM:SS] or [HH:MM:SS]
	re := regexp.MustCompile(`\[(\d{1,2}):(\d{2})(?::(\d{2}))?\]`)
	matches := re.FindAllStringSubmatchIndex(summary, -1)

	for _, match := range matches {
		// Extract timestamp text
		timestampStr := summary[match[0]:match[1]]

		// Extract the sentence following the timestamp (up to the next period or end of text)
		startIndex := match[1]
		endIndex := len(summary)

		nextPeriod := strings.Index(summary[startIndex:], ".")
		if nextPeriod != -1 {
			endIndex = startIndex + nextPeriod + 1 // Include the period
		}

		text := strings.TrimSpace(summary[startIndex:endIndex])

		// Parse time components
		var hours, minutes, seconds int
		timestampComponents := re.FindStringSubmatch(timestampStr)

		if len(timestampComponents) >= 3 {
			fmt.Sscanf(timestampComponents[1], "%d", &minutes)
			fmt.Sscanf(timestampComponents[2], "%d", &seconds)

			if len(timestampComponents) >= 4 && timestampComponents[3] != "" {
				// We have an HH:MM:SS format
				hours = minutes
				minutes = seconds
				fmt.Sscanf(timestampComponents[3], "%d", &seconds)
			}
		}

		// Convert to seconds
		timeInSeconds := hours*3600 + minutes*60 + seconds

		timestamps = append(timestamps, TimestampInfo{
			Time: timeInSeconds,
			Text: text,
		})
	}

	return timestamps
}

// GetFormattedTranscript formats the transcript items into a single string
func GetFormattedTranscript(items []TranscriptItem) string {
	var builder strings.Builder

	for _, item := range items {
		builder.WriteString(fmt.Sprintf("%s %s\n", FormatTimestamp(item.Start), item.Text))
	}

	return strings.TrimSpace(builder.String())
}

// FormatTimestamp converts a float64 timestamp in seconds to [MM:SS] format
func FormatTimestamp(seconds float64) string {
	// Round to nearest second
	totalSeconds := int(seconds)

	// Calculate minutes and remaining seconds
	minutes := totalSeconds / 60
	remainingSeconds := totalSeconds % 60

	// Format as [MM:SS]
	return fmt.Sprintf("[%02d:%02d]", minutes, remainingSeconds)
}
