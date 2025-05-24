package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// UserSummary 구조체는 사용자가 본 비디오 요약의 기록을 나타냅니다.
type UserSummary struct {
	VideoID    string    `json:"video_id"`
	VideoTitle string    `json:"video_title"`
	ViewedAt   time.Time `json:"viewed_at"`
}

// UserSummaries는 사용자의 모든 비디오 요약 기록을 나타냅니다.
type UserSummaries struct {
	UserID    string        `json:"user_id"`
	Summaries []UserSummary `json:"summaries"`
	UpdatedAt time.Time     `json:"updated_at"`
}

var (
	userSummaryMutex sync.RWMutex
	usersDir         = filepath.Join("users")
	maxUserSummaries = 50 // 사용자별 최대 저장 요약 수
)

// InitUserSummaryDirectory는 사용자 요약 디렉토리를 초기화합니다.
func InitUserSummaryDirectory() error {
	// users 디렉토리가 없으면 생성
	if _, err := os.Stat(usersDir); os.IsNotExist(err) {
		err = os.MkdirAll(usersDir, 0755)
		if err != nil {
			return fmt.Errorf("사용자 요약 디렉토리 생성 실패: %w", err)
		}
	}
	return nil
}

// SetMaxUserSummaries는 사용자별 최대 저장 요약 수를 설정합니다.
func SetMaxUserSummaries(max int) {
	if max > 0 {
		maxUserSummaries = max
	}
}

// AddUserSummary는 사용자의 비디오 요약 기록을 추가합니다.
// FIFO 방식으로 최대 개수를 초과하면 가장 오래된 항목을 삭제합니다.
func AddUserSummary(userID, videoID, videoTitle string) error {
	if userID == "" || videoID == "" {
		return fmt.Errorf("사용자 ID와 비디오 ID는 필수입니다")
	}

	userSummaryMutex.Lock()
	defer userSummaryMutex.Unlock()

	// 사용자 요약 파일 경로
	userFilePath := filepath.Join(usersDir, userID+".json")

	// 사용자 요약 목록 로드 또는 생성
	userSummaries := UserSummaries{
		UserID:    userID,
		Summaries: []UserSummary{},
		UpdatedAt: time.Now(),
	}

	// 파일이 존재하면 로드
	if _, err := os.Stat(userFilePath); err == nil {
		file, err := os.Open(userFilePath)
		if err != nil {
			return fmt.Errorf("사용자 요약 파일 열기 실패: %w", err)
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&userSummaries); err != nil {
			return fmt.Errorf("사용자 요약 파일 디코딩 실패: %w", err)
		}
	}

	// 이미 같은 비디오가 있는지 확인하고 중복 제거 (최신 날짜로 업데이트)
	newSummaries := []UserSummary{}
	for _, summary := range userSummaries.Summaries {
		if summary.VideoID != videoID {
			newSummaries = append(newSummaries, summary)
		}
	}

	// 현재 요약 추가
	newSummary := UserSummary{
		VideoID:    videoID,
		VideoTitle: videoTitle,
		ViewedAt:   time.Now(),
	}
	newSummaries = append(newSummaries, newSummary)

	// 요약 수가 최대 개수를 초과하면 가장 오래된 항목 삭제 (FIFO)
	// 최신 항목이 목록의 마지막에 있도록 정렬
	sort.Slice(newSummaries, func(i, j int) bool {
		return newSummaries[i].ViewedAt.Before(newSummaries[j].ViewedAt)
	})

	// 최대 개수를 초과하면 가장 오래된 항목 제거
	if len(newSummaries) > maxUserSummaries {
		newSummaries = newSummaries[len(newSummaries)-maxUserSummaries:]
	}

	userSummaries.Summaries = newSummaries
	userSummaries.UpdatedAt = time.Now()

	// 파일 저장
	file, err := os.Create(userFilePath)
	if err != nil {
		return fmt.Errorf("사용자 요약 파일 생성 실패: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(userSummaries); err != nil {
		return fmt.Errorf("사용자 요약 파일 인코딩 실패: %w", err)
	}

	return nil
}

// GetUserSummaries는 사용자의 비디오 요약 기록을 가져옵니다.
// limit이 0보다 크면 최신 항목 limit개만 반환합니다.
func GetUserSummaries(userID string, limit int) ([]UserSummary, error) {
	if userID == "" {
		return nil, fmt.Errorf("사용자 ID는 필수입니다")
	}

	userSummaryMutex.RLock()
	defer userSummaryMutex.RUnlock()

	// 사용자 요약 파일 경로
	userFilePath := filepath.Join(usersDir, userID+".json")

	// 파일이 존재하지 않으면 빈 목록 반환
	if _, err := os.Stat(userFilePath); os.IsNotExist(err) {
		return []UserSummary{}, nil
	}

	// 파일 로드
	file, err := os.Open(userFilePath)
	if err != nil {
		return nil, fmt.Errorf("사용자 요약 파일 열기 실패: %w", err)
	}
	defer file.Close()

	var userSummaries UserSummaries
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&userSummaries); err != nil {
		return nil, fmt.Errorf("사용자 요약 파일 디코딩 실패: %w", err)
	}

	// 요약 목록 시간 순으로 정렬 (최신 항목이 먼저 오도록)
	sort.Slice(userSummaries.Summaries, func(i, j int) bool {
		return userSummaries.Summaries[i].ViewedAt.After(userSummaries.Summaries[j].ViewedAt)
	})

	// limit이 0보다 크면 최신 항목 limit개만 반환
	if limit > 0 && limit < len(userSummaries.Summaries) {
		return userSummaries.Summaries[:limit], nil
	}

	return userSummaries.Summaries, nil
}

// GetRecentUserSummaries는 사용자의 최근 15개 요약을 가져옵니다.
func GetRecentUserSummaries(userID string) ([]UserSummary, error) {
	// 최근 15개 요약 가져오기
	return GetUserSummaries(userID, 15)
}
