package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// VideoInfo holds basic information about a YouTube video
type VideoInfo struct {
	ID         string
	Title      string
	Channel    string
	UploadDate string
	Duration   int
}

// TranscriptItem represents a single transcript item with text and timestamp
type TranscriptItem struct {
	Text     string  `json:"text"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// GetVideoID extracts the video ID from a YouTube URL
func GetVideoID(videoURL string) (string, error) {
	// Regular expressions for different YouTube URL formats
	patterns := []string{
		`(?:youtube\.com\/watch\?v=|youtu.be\/)([^&\?\/]+)`,
		`youtube\.com\/embed\/([^\/\?]+)`,
		`youtube\.com\/v\/([^\/\?]+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(videoURL)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", errors.New("invalid YouTube URL")
}

// GetVideoInfo fetches basic information about a YouTube video using yt-dlp
func GetVideoInfo(videoID string) (*VideoInfo, error) {
	// Validate the video ID to prevent command injection
	validIDPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	if !validIDPattern.MatchString(videoID) {
		return nil, errors.New("invalid video ID format")
	}

	// Construct YouTube URL from video ID
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	// Prepare yt-dlp command to get video info in JSON format
	cmd := exec.Command(
		"yt-dlp",
		"--dump-json",
		"--no-playlist",
		"--skip-download",
		videoURL,
	)

	// Capture stdout
	var out bytes.Buffer
	cmd.Stdout = &out

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp error: %v - %s", err, stderr.String())
	}

	// Parse the JSON output
	var videoData map[string]interface{}
	err = json.Unmarshal(out.Bytes(), &videoData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp output: %v", err)
	}

	// Extract relevant information
	title, _ := videoData["title"].(string)
	channel, _ := videoData["channel"].(string)
	uploadDate, _ := videoData["upload_date"].(string)

	// Parse duration (can be a string or a float)
	var duration int
	switch d := videoData["duration"].(type) {
	case float64:
		duration = int(d)
	case string:
		f, err := strconv.ParseFloat(d, 64)
		if err == nil {
			duration = int(f)
		}
	default:
		duration = 0
	}

	return &VideoInfo{
		ID:         videoID,
		Title:      title,
		Channel:    channel,
		UploadDate: uploadDate,
		Duration:   duration,
	}, nil
}

// GetTranscript fetches the transcript for a YouTube video using yt-dlp
// Add a new parameter chunkSize to specify the size of each chunk in seconds
func GetTranscript(videoID string, chunkSize float64) ([][]TranscriptItem, error) {
	// Validate the video ID to prevent command injection
	validIDPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	if !validIDPattern.MatchString(videoID) {
		return nil, errors.New("invalid video ID format")
	}

	// Create a temporary directory for subtitle files
	tempDir, err := os.MkdirTemp("", "yt-subtitles-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up temp directory when done

	// Construct YouTube URL from video ID
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	// Prepare yt-dlp command to get subtitles
	cmd := exec.Command(
		"yt-dlp",
		"--write-sub",       // Try to get manual subtitles
		"--write-auto-sub",  // Get auto-generated subtitles if no manual subs available
		"--sub-langs", "ko", // Prioritize Korean subtitles
		"--skip-download",     // Don't download the video
		"--sub-format", "vtt", // Get WebVTT format
		"--paths", tempDir, // Save subtitle files to our temp directory
		"-o '%(id)s.%(ext)s'",
		videoURL,
	)

	// Capture stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run the command
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp failed to download subtitles: %v - %s", err, stderr.String())
	}

	// Process subtitle files and split them into chunks
	return processSubtitleFiles(tempDir, chunkSize)
}

// Extracts and processes subtitle files from a temporary directory
func processSubtitleFiles(tempDir string, chunkSize float64) ([][]TranscriptItem, error) {
	// Read files from the temp directory
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read temp directory: %v", err)
	}

	if len(files) == 0 {
		return nil, errors.New("no subtitle files were downloaded")
	}

	// Process each subtitle file and collect transcript items
	var allTranscriptItems []TranscriptItem
	for _, file := range files {
		// Only process .vtt files
		if !strings.HasSuffix(file.Name(), ".vtt") {
			continue
		}

		// Read the subtitle file
		filePath := fmt.Sprintf("%s/%s", tempDir, file.Name())
		subtitleData, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files we can't read
		}

		// Process the VTT content
		transcriptItems := parseVttContent(string(subtitleData))
		allTranscriptItems = append(allTranscriptItems, transcriptItems...)
	}

	// Check if we actually got any transcript items
	if len(allTranscriptItems) == 0 {
		return nil, errors.New("no usable transcript entries were found")
	}

	// Sort transcript items by start time
	sortTranscriptItemsByTime(allTranscriptItems)

	// Merge consecutive transcript items that are very close in time
	// allTranscriptItems = MergeConsecutiveTranscriptItems(allTranscriptItems)

	// Split transcript items into chunks
	var chunks [][]TranscriptItem
	var currentChunk []TranscriptItem
	var currentChunkStart float64

	for _, item := range allTranscriptItems {
		if len(currentChunk) == 0 {
			currentChunkStart = item.Start
		}

		if item.Start-currentChunkStart < chunkSize {
			currentChunk = append(currentChunk, item)
		} else {
			chunks = append(chunks, currentChunk)
			currentChunk = []TranscriptItem{item}
			currentChunkStart = item.Start
		}
	}

	// Add the last chunk if it exists
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks, nil
}

// parseVttContent converts VTT content to TranscriptItem array
func parseVttContent(vttContent string) []TranscriptItem {
	var transcriptItems []TranscriptItem

	// Check if it has at least a basic VTT structure
	lines := strings.Split(vttContent, "\n")
	if len(lines) < 4 || !strings.Contains(lines[0], "WEBVTT") {
		return transcriptItems
	}

	// Skip the header lines (usually first 4 lines including WEBVTT, empty line, etc.)
	contentLines := lines[4:]

	// Process the content lines
	var currentText strings.Builder
	var startTime float64
	var endTime float64

	for i := 0; i < len(contentLines); i++ {
		line := contentLines[i]

		// Process timestamp lines
		if strings.Contains(line, "-->") {
			// If we have collected text from previous timestamps, save it
			if currentText.Len() > 0 {
				text := cleanTranscriptText(currentText.String())
				if text != "" {
					transcriptItems = append(transcriptItems, TranscriptItem{
						Text:     text,
						Start:    startTime,
						Duration: endTime - startTime,
					})
				}
				currentText.Reset()
			}

			// Parse new timestamps
			timestamps := strings.Split(line, "-->")
			if len(timestamps) == 2 {
				startTime = parseVttTimestamp(strings.TrimSpace(timestamps[0]))
				endTime = parseVttTimestamp(strings.TrimSpace(timestamps[1]))
			}
			continue
		}

		// Skip positioning metadata lines
		if strings.Contains(line, "align:") || strings.Contains(line, "position:") {
			continue
		}

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Clean up the line by removing timestamp tags
		cleanedLine := cleanVttLine(line)
		if cleanedLine != "" {
			if currentText.Len() > 0 {
				currentText.WriteString(" ")
			}
			currentText.WriteString(cleanedLine)
		}
	}

	// Don't forget to add the last collected text if any
	if currentText.Len() > 0 {
		text := cleanTranscriptText(currentText.String())
		if text != "" {
			transcriptItems = append(transcriptItems, TranscriptItem{
				Text:     text,
				Start:    startTime,
				Duration: endTime - startTime,
			})
		}
	}

	return transcriptItems
}

// cleanVttLine removes timestamp tags and other artifacts from VTT lines
func cleanVttLine(line string) string {
	// Remove timestamp tags like <00:00:07.759>
	cleanedLine := regexp.MustCompile(`<\d{2}:\d{2}:\d{2}\.\d{3}>`).ReplaceAllString(line, "")

	// Remove other VTT specific tags
	cleanedLine = regexp.MustCompile(`</?c>`).ReplaceAllString(cleanedLine, "")

	return strings.TrimSpace(cleanedLine)
}

// parseVttTimestamp converts VTT timestamp (00:00:00.000) to seconds as float64
func parseVttTimestamp(timestamp string) float64 {
	// Handle timestamps like "00:00:07.759"
	parts := strings.Split(timestamp, ":")
	if len(parts) != 3 {
		return 0
	}

	// Split seconds and milliseconds
	secParts := strings.Split(parts[2], ".")
	if len(secParts) != 2 {
		return 0
	}

	// Parse hours, minutes, seconds and milliseconds
	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(secParts[0])
	milliseconds, _ := strconv.Atoi(secParts[1])

	// Convert to seconds
	return float64(hours*3600+minutes*60+seconds) + float64(milliseconds)/1000
}

// sortTranscriptItemsByTime sorts the transcript items by their start time
func sortTranscriptItemsByTime(items []TranscriptItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Start < items[j].Start
	})
}

// cleanTranscriptText removes common artifacts from subtitle text
func cleanTranscriptText(text string) string {
	// Skip if empty
	if text == "" {
		return ""
	}

	// Remove HTML tags
	htmlTagRegex := regexp.MustCompile("<[^>]*>")
	text = htmlTagRegex.ReplaceAllString(text, "")

	// Remove multiple spaces
	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")

	// Remove common subtitle artifacts (like musical notes, speaker identifications)
	artifactRegex := regexp.MustCompile(`\[.*?\]|\(.*?\)|\{.*?\}`)
	text = artifactRegex.ReplaceAllString(text, "")

	return strings.TrimSpace(text)
}
