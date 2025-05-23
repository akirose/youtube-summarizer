package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/akirose/youtube-summarizer/services"
)

// SummaryCache represents the cache for video summaries
type SummaryCache struct {
	mutex    sync.RWMutex
	cacheDir string
	items    map[string]*CacheItem
}

// CacheItem represents a single cache item
type CacheItem struct {
	VideoID    string                    `json:"videoId"`
	Title      string                    `json:"title"`
	Summary    string                    `json:"summary"`
	Timestamps []Timestamp               `json:"timestamps"`
	Transcript []services.TranscriptItem `json:"transcript,omitempty"` // 트랜스크립트 데이터 저장
	CreatedAt  time.Time                 `json:"createdAt"`
}

// Timestamp represents a timestamp in the summary
type Timestamp struct {
	Time int    `json:"time"`
	Text string `json:"text"`
}

// VideoSummary represents the schema for storing video titles and summaries
type VideoSummary struct {
	VideoTitle string `json:"video_title"` // Title of the video
	VideoID    string `json:"video_id"`    // Video ID
}

// GetRecentVideoSummaries retrieves the most recent 10 VideoSummary entries
// Updated to include recent files from the cache directory
func GetRecentVideoSummaries() []VideoSummary {
	// Fetch all JSON files in the cache directory
	files, err := filepath.Glob(filepath.Join("cache", "*.json"))
	if err != nil {
		fmt.Printf("Warning: Failed to list cache files: %v\n", err)
		return nil
	}

	// Sort files by modification time in descending order
	sort.Slice(files, func(i, j int) bool {
		infoI, errI := os.Stat(files[i])
		infoJ, errJ := os.Stat(files[j])
		if errI != nil || errJ != nil {
			return false
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// Limit to the most recent 15 files
	if len(files) > 15 {
		files = files[:15]
	}

	// Read and parse each file into VideoSummary
	var recentSummaries []VideoSummary
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			fmt.Printf("Warning: Failed to open cache file %s: %v\n", file, err)
			continue
		}
		defer f.Close()

		var item CacheItem
		decoder := json.NewDecoder(f)
		if err := decoder.Decode(&item); err != nil {
			fmt.Printf("Warning: Failed to decode cache file %s: %v\n", file, err)
			continue
		}

		recentSummaries = append(recentSummaries, VideoSummary{
			VideoTitle: item.Title,
			VideoID:    item.VideoID,
		})
	}

	return recentSummaries
}

// NewSummaryCache creates a new cache
func NewSummaryCache(cacheDir string) (*SummaryCache, error) {
	// Create cache directory if it doesn't exist
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	cache := &SummaryCache{
		cacheDir: cacheDir,
		items:    make(map[string]*CacheItem),
	}

	// Load existing cache items
	if err := cache.loadFromDisk(); err != nil {
		fmt.Printf("Warning: Failed to load cache from disk: %v\n", err)
	}

	return cache, nil
}

// Get retrieves an item from the cache
func (c *SummaryCache) Get(videoID string) (*CacheItem, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	item, ok := c.items[videoID]
	return item, ok
}

// Set adds an item to the cache
func (c *SummaryCache) Set(videoID, title, summary string, timestamps []Timestamp, transcript []services.TranscriptItem) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	item := &CacheItem{
		VideoID:    videoID,
		Title:      title,
		Summary:    summary,
		Timestamps: timestamps,
		Transcript: transcript,
		CreatedAt:  time.Now(),
	}

	c.items[videoID] = item

	// Save to disk
	return c.saveToDisk(videoID, item)
}

// Delete removes an item from the cache
func (c *SummaryCache) Delete(videoID string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Check if item exists
	if _, ok := c.items[videoID]; !ok {
		return nil
	}

	// Remove from memory
	delete(c.items, videoID)

	// Remove from disk
	filename := filepath.Join(c.cacheDir, videoID+".json")
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}

	return nil
}

// Clear removes all items from the cache
func (c *SummaryCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Clear memory cache
	c.items = make(map[string]*CacheItem)

	// Remove all files from cache directory
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			fmt.Printf("Warning: Failed to remove cache file %s: %v\n", file, err)
		}
	}

	return nil
}

// saveToDisk saves a cache item to disk
func (c *SummaryCache) saveToDisk(videoID string, item *CacheItem) error {
	// Create cache file
	filename := filepath.Join(c.cacheDir, videoID+".json")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	// Write cache item to file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(item); err != nil {
		return fmt.Errorf("failed to encode cache item: %w", err)
	}

	return nil
}

// loadFromDisk loads cache items from disk
func (c *SummaryCache) loadFromDisk() error {
	// Find all cache files
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files: %w", err)
	}

	// Load each file
	for _, file := range files {
		// Extract video ID from filename
		videoID := filepath.Base(file)
		videoID = videoID[:len(videoID)-5] // Remove .json extension

		// Open file
		f, err := os.Open(file)
		if err != nil {
			fmt.Printf("Warning: Failed to open cache file %s: %v\n", file, err)
			continue
		}

		// Decode file
		var item CacheItem
		decoder := json.NewDecoder(f)
		if err := decoder.Decode(&item); err != nil {
			f.Close()
			fmt.Printf("Warning: Failed to decode cache file %s: %v\n", file, err)
			continue
		}
		f.Close()

		// Add to memory cache
		c.items[videoID] = &item
	}

	return nil
}

// AddUserSummaryToCache는 캐시에 비디오 요약을 추가하고 동시에 사용자의 요약 목록에도 추가합니다.
func (c *SummaryCache) AddUserSummaryToCache(userID, videoID, title, summary string, timestamps []Timestamp, transcript []services.TranscriptItem) error {
	// 먼저 글로벌 캐시에 추가
	err := c.Set(videoID, title, summary, timestamps, transcript)
	if err != nil {
		return fmt.Errorf("글로벌 캐시에 추가 실패: %w", err)
	}

	// 사용자의 요약 목록에 추가
	err = AddUserSummary(userID, videoID, title)
	if err != nil {
		return fmt.Errorf("사용자 요약 목록에 추가 실패: %w", err)
	}

	return nil
}
