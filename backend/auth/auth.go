package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	googleOAuthConfig *oauth2.Config
	// 세션 관리를 위한 맵과 뮤텍스
	sessions     = make(map[string]*Session)
	sessionMutex sync.RWMutex
)

// UserInfo는 Google에서 반환된 사용자 정보를 저장하는 구조체
type UserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}

// Session은 사용자 세션을 저장하는 구조체
type Session struct {
	ID           string    `json:"id"`
	UserInfo     *UserInfo `json:"user_info"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// InitAuth OAuth 설정을 초기화합니다
func InitAuth() {
	clientID := os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	redirectURL := os.Getenv("GOOGLE_OAUTH_REDIRECT_URI")

	if clientID == "" || clientSecret == "" {
		log.Println("Warning: Google OAuth credentials not set in environment variables")
		return
	}

	if redirectURL == "" {
		redirectURL = "http://localhost:8080/auth/google/callback"
	}

	googleOAuthConfig = &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	// 주기적으로 만료된 세션 정리
	go cleanupExpiredSessions()
}

// 만료된 세션을 주기적으로 정리하는 함수
func cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		sessionMutex.Lock()
		now := time.Now()
		for id, session := range sessions {
			if now.After(session.ExpiresAt) {
				delete(sessions, id)
				log.Printf("Expired session cleaned up: %s", id)
			}
		}
		sessionMutex.Unlock()
	}
}

// GoogleLoginHandler는 Google OAuth 로그인 프로세스를 시작합니다
func GoogleLoginHandler(c *gin.Context) {
	if googleOAuthConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OAuth not configured"})
		return
	}

	// 상태 파라미터를 설정하여 CSRF 공격 방지
	stateToken := uuid.New().String()
	c.SetCookie("oauth_state", stateToken, 3600, "/", "", false, true)
	url := googleOAuthConfig.AuthCodeURL(stateToken, oauth2.AccessTypeOffline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GoogleCallbackHandler는 Google OAuth 콜백을 처리합니다
func GoogleCallbackHandler(c *gin.Context) {
	if googleOAuthConfig == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OAuth not configured"})
		return
	}

	// 인증 코드 획득
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code not provided"})
		return
	}

	// 상태 토큰 확인 (보안을 위한 CSRF 방지)
	state := c.Query("state")
	storedState, _ := c.Cookie("oauth_state")
	if state == "" || state != storedState {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid state token"})
		return
	}

	// 코드를 토큰으로 교환
	token, err := googleOAuthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange token"})
		return
	}

	// Google API에서 사용자 정보 가져오기
	userInfo, err := getUserInfo(token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
		return
	}

	// 새 세션 생성
	session := &Session{
		ID:           uuid.New().String(),
		UserInfo:     userInfo,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry,
		CreatedAt:    time.Now(),
	}

	// 세션 저장
	sessionMutex.Lock()
	sessions[session.ID] = session
	sessionMutex.Unlock()

	// 세션 ID를 쿠키에 설정
	c.SetCookie("session_id", session.ID, 3600*24*7, "/", "", false, true)

	// 사용자 정보를 클라이언트로 전달
	c.HTML(http.StatusOK, "callback.html", gin.H{
		"userInfo": userInfo,
		"token":    session.ID, // 액세스 토큰 대신 세션 ID 반환
	})
}

// GetSessionUser는 요청의 쿠키에서 세션 ID를 추출하고 해당 사용자 정보를 반환합니다
func GetSessionUser(c *gin.Context) (*UserInfo, bool) {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		return nil, false
	}

	sessionMutex.RLock()
	session, exists := sessions[sessionID]
	sessionMutex.RUnlock()

	if !exists || time.Now().After(session.ExpiresAt) {
		// 세션이 존재하지 않거나 만료된 경우
		return nil, false
	}

	return session.UserInfo, true
}

// RefreshSession은 필요한 경우 세션을 갱신합니다
func RefreshSession(c *gin.Context) bool {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		return false
	}

	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session, exists := sessions[sessionID]
	if !exists {
		return false
	}

	// 세션 만료 시간 확인 - 만료 1시간 전부터 갱신
	if time.Now().Add(1*time.Hour).After(session.ExpiresAt) && session.RefreshToken != "" {
		// OAuth 토큰 갱신 시도
		token, err := googleOAuthConfig.TokenSource(c.Request.Context(), &oauth2.Token{
			RefreshToken: session.RefreshToken,
		}).Token()

		if err != nil {
			log.Printf("Failed to refresh token: %v", err)
			return false
		}

		// 새로운 정보로 세션 업데이트
		session.AccessToken = token.AccessToken
		session.ExpiresAt = token.Expiry

		// 새 세션 정보로 쿠키 갱신
		c.SetCookie("session_id", session.ID, 3600*24*7, "/", "", false, true)
	}

	return true
}

// IsAuthenticated는 사용자가 인증되었는지 확인합니다
func IsAuthenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		userInfo, authenticated := GetSessionUser(c)
		if !authenticated {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		// 세션 갱신 (필요한 경우)
		RefreshSession(c)

		// 사용자 정보를 컨텍스트에 추가
		sessionData := map[string]interface{}{
			"userId":  userInfo.ID,
			"email":   userInfo.Email,
			"name":    userInfo.Name,
			"picture": userInfo.Picture,
		}
		c.Set("session", sessionData)

		c.Next()
	}
}

// LogoutHandler는 사용자의 세션을 종료합니다
func LogoutHandler(c *gin.Context) {
	// 세션 ID 가져오기
	sessionID, err := c.Cookie("session_id")
	if err == nil {
		// 세션 맵에서 제거
		sessionMutex.Lock()
		delete(sessions, sessionID)
		sessionMutex.Unlock()
	}

	// 쿠키 삭제
	c.SetCookie("session_id", "", -1, "/", "", false, true)
	c.SetCookie("oauth_state", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Successfully logged out"})
}

// OAuth 액세스 토큰을 사용하여 사용자 정보를 가져옵니다
func getUserInfo(accessToken string) (*UserInfo, error) {
	// Google 사용자 정보 API 호출
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: %s", resp.Status)
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}
