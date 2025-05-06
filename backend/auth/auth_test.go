package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestLogoutHandler는 로그아웃 핸들러가 제대로 작동하는지 테스트합니다.
func TestLogoutHandler(t *testing.T) {
	// Gin 테스트 모드 설정
	gin.SetMode(gin.TestMode)

	// 테스트용 라우터 생성
	router := gin.New()
	router.POST("/auth/logout", LogoutHandler)

	// 테스트 요청 생성
	req, err := http.NewRequest("POST", "/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}

	// 쿠키 추가 (테스트를 위한 세션 ID)
	req.AddCookie(&http.Cookie{
		Name:  "session_id",
		Value: "test-session-id",
	})

	// 응답 레코더 생성
	w := httptest.NewRecorder()

	// 테스트 세션 추가
	sessionMutex.Lock()
	sessions["test-session-id"] = &Session{ID: "test-session-id"}
	sessionMutex.Unlock()

	// 핸들러 호출
	router.ServeHTTP(w, req)

	// 응답 코드 검증
	assert.Equal(t, http.StatusOK, w.Code)

	// 세션이 제대로 제거되었는지 확인
	sessionMutex.RLock()
	_, exists := sessions["test-session-id"]
	sessionMutex.RUnlock()
	assert.False(t, exists, "Session should be removed after logout")

	// 쿠키가 제대로 설정되었는지 확인
	cookies := w.Result().Cookies()

	// session_id 쿠키가 만료되었는지 확인
	var sessionCookie *http.Cookie
	var oauthStateCookie *http.Cookie

	for _, cookie := range cookies {
		if cookie.Name == "session_id" {
			sessionCookie = cookie
		} else if cookie.Name == "oauth_state" {
			oauthStateCookie = cookie
		}
	}

	// 쿠키 확인
	assert.NotNil(t, sessionCookie, "session_id cookie should be present")
	assert.NotNil(t, oauthStateCookie, "oauth_state cookie should be present")

	// 쿠키가 만료되었는지 확인 (MaxAge < 0)
	assert.Less(t, sessionCookie.MaxAge, 0, "session_id cookie should be expired")
	assert.Less(t, oauthStateCookie.MaxAge, 0, "oauth_state cookie should be expired")
}
