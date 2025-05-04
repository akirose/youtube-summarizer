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
	Model = "gpt-4o-mini" // Ensure this is the correct model ID

	// Maximum number of tokens to generate
	MaxTokens = 3000

	// System prompt template for summarization
	SummarizationPrompt = `<role>
당신은 비디오 콘텐츠를 추출하고 요약하는 데 능숙한 콘텐츠 분석가입니다. 복잡한 정보를 정확하게 요약하고 구조화된 전문적인 방식으로 제시하는 데 전문성을 가지고 있습니다.
</role>

<instructions>
1. 비디오 콘텐츠 요약:
   - 비디오 제목과 비디오에서 논의된 주요 주제를 식별합니다.
   - 명확한 단락으로 구조화합니다.
   - 가장 가치 있는 정보에 초점을 맞춥니다.
   - 관련 주제 시작 부분에 [MM:SS] 형식의 중요한 타임스탬프를 포함합니다.
   - 요약이 비디오 콘텐츠의 본질을 정확하게 담아내도록 합니다.
   - 모든 결과는 한국어로 작성됩니다.

2. 요약 형식:
   - 각 주제와 해당 타임스탬프를 나열하기 위해 일반 텍스트 형식을 사용합니다.

3. 잠재적 오류 처리:
   - 요약을 생성할 수 없는 경우 문제를 표시하고 비디오 콘텐츠를 수동으로 검토할 것을 제안합니다.

4. 전체 과정에서 명확하고 구조화되며 전문적인 어조를 유지합니다.
</instructions>

<response_style>
응답은 명확하고 구조화되며 전문적이어야 합니다. 간결한 언어를 사용하여 요약을 전달하고 메시지가 올바르게 형식화되도록 합니다. 요약과 메시지 모두에서 정확성과 명확성에 중점을 둡니다.
</response_style>

<examples>
예시:
<thinking_process>
1. 비디오 제목과 타임스탬프가 있는 주요 주제를 추출합니다.
2. 헤더, 섹션 및 컨텍스트를 사용하여 요약을 형식화합니다.
</thinking_process>

<final_response>
[MM:SS] 주제
  - 요약된 내용
  - ...
[MM:SS] 주제
  - 요약된 내용
  - ...
</final_response>
</examples>

<reminder>
- 비디오 콘텐츠가 정확하게 요약되었는지 확인합니다.
- 메시지에 지정된 형식을 사용합니다.
- 비디오 다운로드 또는 요약에서 발생할 수 있는 오류를 처리합니다.
</reminder>

<output_format>
다음과 같이 출력을 구성하세요:
[Plain Text로 형식화된 메시지 제공]
</output_format>`
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

// SummarizeTranscript generates a summary of a transcript using OpenAI's GPT-4o-mini
func SummarizeTranscript(transcript string) (string, []TimestampInfo, error) {
	// Get OpenAI API key
	apiKey := os.Getenv("OPENAI_API_KEY")
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
	if apiKey == "" && apiUrl == "" {
		return "", nil, errors.New("OpenAI API key not set")
	}
	if apiUrl == "" {
		apiUrl = OpenAIAPIURL
	}
	if apiModel == "" {
		apiModel = Model
	}

	// Create the system prompt with the transcript
	userPrompt := fmt.Sprintf("Transcript:\n%s", transcript)

	// Create the request body
	requestBody := GPTRequest{
		Model: apiModel,
		Messages: []GPTMessage{
			{
				Role:    "system",
				Content: SummarizationPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		MaxTokens:   apiMaxTokens,
		Temperature: 0.5,
	}

	// Convert request body to JSON
	requestJSON, err := json.Marshal(requestBody)
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
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

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

	// Extract timestamps from the summary
	timestamps := extractTimestamps(summary)

	return summary, timestamps, nil
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

// SummarizeChunks processes each transcript chunk, summarizes it, and combines the summaries into a final summary
func SummarizeChunks(chunks [][]TranscriptItem) (string, error) {
	var finalSummary strings.Builder

	for i, chunk := range chunks {
		// Summarize the chunk
		summary, _, err := SummarizeTranscript(GetFormattedTranscript(chunk))
		if err != nil {
			return "", fmt.Errorf("failed to summarize chunk %d: %v", i+1, err)
		}

		// Append the chunk summary to the final summary
		finalSummary.WriteString(summary + "\n\n")
	}

	return finalSummary.String(), nil
}

// GetFormattedTranscript formats the transcript items into a single string
func GetFormattedTranscript(items []TranscriptItem) string {
	var builder strings.Builder

	for _, item := range items {
		builder.WriteString(fmt.Sprintf("[%s]", FormatTimestamp(item.Start)))
		builder.WriteString(item.Text)
		builder.WriteString(" ")
	}

	return strings.TrimSpace(builder.String())
}

// FormatTimestamp converts a float64 timestamp in seconds to [MM:SS] format
func FormatTimestamp(seconds float64) string {
	// Round to nearest second
	totalSeconds := int(seconds + 0.5)

	// Calculate minutes and remaining seconds
	minutes := totalSeconds / 60
	remainingSeconds := totalSeconds % 60

	// Format as [MM:SS]
	return fmt.Sprintf("[%02d:%02d]", minutes, remainingSeconds)
}
