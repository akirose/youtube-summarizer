package services

import (
	"os"
	"strings"
	"sync"
)

// API 키 정책 상수
const (
	// 모든 사용자가 서버 키 사용 가능
	PolicyAllUsers = "all"
	// 지정된 사용자만 서버 키 사용 가능
	PolicyDesignatedUsers = "designated"
)

// APIKeyPolicy는 OpenAI API 키 사용 정책을 관리하는 구조체
type APIKeyPolicy struct {
	// 서버 API 키 사용 정책 (PolicyAllUsers 또는 PolicyDesignatedUsers)
	Policy string
	// 지정된 사용자 ID 목록 (PolicyDesignatedUsers인 경우 사용)
	DesignatedUsers map[string]bool
	mu              sync.RWMutex
}

var (
	// 전역 정책 인스턴스
	globalPolicy *APIKeyPolicy
	once         sync.Once
)

// InitAPIKeyPolicy initializes the API key policy from environment variables
func InitAPIKeyPolicy() *APIKeyPolicy {
	once.Do(func() {
		globalPolicy = &APIKeyPolicy{
			Policy:          PolicyAllUsers, // 기본값: 모든 사용자가 사용 가능
			DesignatedUsers: make(map[string]bool),
		}

		// 환경 변수에서 정책 읽기
		policy := os.Getenv("SERVER_OPENAI_API_KEY_POLICY")
		if policy == PolicyDesignatedUsers {
			globalPolicy.Policy = PolicyDesignatedUsers
		}

		// 지정된 사용자 ID 읽기 (쉼표로 구분된 목록)
		designatedUsers := os.Getenv("DESIGNATED_USERS")
		if designatedUsers != "" {
			userIDs := strings.Split(designatedUsers, ",")
			for _, userID := range userIDs {
				globalPolicy.DesignatedUsers[strings.TrimSpace(userID)] = true
			}
		}
	})

	return globalPolicy
}

// GetAPIKeyPolicy returns the global API key policy instance
func GetAPIKeyPolicy() *APIKeyPolicy {
	if globalPolicy == nil {
		return InitAPIKeyPolicy()
	}
	return globalPolicy
}

// CanUseServerKey checks if a user can use the server's OpenAI API key
func (p *APIKeyPolicy) CanUseServerKey(userID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.Policy == PolicyAllUsers {
		return true
	}

	// PolicyDesignatedUsers인 경우 지정된 사용자 목록에 있는지 확인
	return p.DesignatedUsers[userID]
}

// UpdateDesignatedUsers updates the list of designated users
func (p *APIKeyPolicy) UpdateDesignatedUsers(userIDs []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 기존 목록 초기화
	p.DesignatedUsers = make(map[string]bool)

	// 새 목록 설정
	for _, userID := range userIDs {
		p.DesignatedUsers[strings.TrimSpace(userID)] = true
	}
}

// GetApiKeyPolicy returns the current policy as a string
func (p *APIKeyPolicy) GetApiKeyPolicy() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Policy
}
