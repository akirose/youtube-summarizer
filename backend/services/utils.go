package services

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// GetEnvBool reads a boolean environment variable
func GetEnvBool(key string, fallback bool) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}

	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return boolValue
}

// GetEnvInt reads an integer environment variable
func GetEnvInt(key string, fallback int) int {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return intValue
}

// FormatDuration formats seconds into a human-readable duration string (MM:SS or HH:MM:SS)
func FormatDuration(seconds int) string {
	duration := time.Duration(seconds) * time.Second
	
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	secs := int(duration.Seconds()) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// SanitizeString cleans a string for safer usage
func SanitizeString(input string) string {
	// Replace newlines with spaces
	result := strings.ReplaceAll(input, "\n", " ")
	
	// Replace tabs with spaces
	result = strings.ReplaceAll(result, "\t", " ")
	
	// Replace multiple spaces with a single space
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	
	return strings.TrimSpace(result)
}

// TruncateString truncates a string to a maximum length and adds an ellipsis if needed
func TruncateString(input string, maxLength int) string {
	if len(input) <= maxLength {
		return input
	}
	
	// Truncate to maxLength-3 to account for the ellipsis
	return input[:maxLength-3] + "..."
}

// ChunkText splits a long text into manageable chunks for API requests
func ChunkText(text string, maxChunkSize int) []string {
	var chunks []string
	
	// If the text is already smaller than the max chunk size, return it as is
	if len(text) <= maxChunkSize {
		return []string{text}
	}
	
	// Split the text by sentences (roughly)
	sentences := strings.Split(text, ". ")
	
	currentChunk := ""
	for i, sentence := range sentences {
		// Add the period back except for the last sentence if it doesn't already end with one
		if i < len(sentences)-1 || !strings.HasSuffix(sentence, ".") {
			sentence += "."
		}
		
		// If adding this sentence would exceed the max chunk size, start a new chunk
		if len(currentChunk)+len(sentence)+1 > maxChunkSize && currentChunk != "" {
			chunks = append(chunks, strings.TrimSpace(currentChunk))
			currentChunk = sentence + " "
		} else {
			currentChunk += sentence + " "
		}
	}
	
	// Add the last chunk if not empty
	if strings.TrimSpace(currentChunk) != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}
	
	return chunks
}
