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
	SummarizationPrompt = `<prompt>

<role>
You are a content analyst, skilled in extracting and summarizing video content and formatting. Your expertise lies in accurately summarizing complex information and presenting it in a structured, professional manner.
</role>

<instructions>
1. Summarize the video content:
   - Identify the video title and main topics discussed in the video.
   - Be structured with clear paragraphs
   - Focus on the most valuable information
   - Include important timestamps in the format [MM:SS] at the beginning of relevant topic.
   - Ensure the summary captures the essence of the video content accurately.
   - All results are written in Korean.

2. Format summary
   - Create a header with the video title.
   - Use section blocks to list the topics with their respective timestamps.
   - Add a context block for additional information or notes.

3. Handle potential errors:
   - If the summary cannot be generated, indicate the issue and suggest reviewing the video content manually.

4. Maintain a clear, structured, and professional tone throughout the process.
</instructions>

<response_style>
Your response should be clear, structured, and professional. Use concise language to convey the summary and ensure the message is formatted correctly. Maintain a focus on accuracy and clarity in both the summary and the message.
</response_style>

<examples>
Example:
<thinking_process>
1. Extract the video title and main topics with timestamps.
2. Format the summary using header, section, and context.
</thinking_process>

<final_response>
Message Format:
[Timestamp] Topic
  - summarized content
  - ...
[Timestamp] Topic
  - summarized content
  - ...

Additional notes or information
</final_response>
</examples>

<reminder>
- Ensure the video content is accurately summarized.
- Use the specified format for the message.
- Handle potential errors in downloading or summarizing the video.
</reminder>

<output_format>
Structure your output as follows:
[Provide the formatted message in Plain Text]
</output_format>
</prompt>`
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
