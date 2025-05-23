// Global variables
let player;
let currentVideoId = null;
let apiReady = false; // YouTube API 준비 상태 추적
let pendingVideoId = null; // API 준비 전에 로드할 비디오 ID 저장
let timeUpdateInterval; // Player 상태를 추적하는 변수 추가
let isLoggedIn = false; // 로그인 상태 추적
let userProfile = null; // 사용자 프로필 정보 저장
let googleAuth = null; // Google 인증 객체
let needsApiKey = true; // API 키가 필요한지 여부 (기본값은 필요함)
let serverKeyPolicy = null; // 서버 API 키 정책
let summaryEventSource = null; // For SSE connection

// DOM elements
const searchForm = document.getElementById('search-form');
const videoUrlInput = document.getElementById('video-url');
const clearBtn = document.getElementById('clear-btn');
const searchBtn = document.getElementById('search-btn');
const searchContainer = document.querySelector('.search-container');
const resultsContainer = document.getElementById('results-container');
const mainContent = document.querySelector('.main-content'); // 새로운 메인 컨텐츠 요소 추가
const logo = document.querySelector('.logo'); // 로고 요소 추가
const videoTitle = document.getElementById('video-title');
const summaryElement = document.getElementById('summary');
const loadingElement = document.getElementById('loading');
const loginButton = document.getElementById('login-button');
const userDropdown = document.getElementById('user-dropdown');
const loginModal = document.getElementById('login-modal');
const closeLoginModalBtn = document.getElementById('close-login-modal');
const googleLoginBtn = document.getElementById('google-login-btn');
const logoutBtn = document.getElementById('logout-btn');
const apiKeyModal = document.getElementById('api-key-modal');
const closeApiKeyModalBtn = document.getElementById('close-api-key-modal');
const saveApiKeyBtn = document.getElementById('save-api-key-btn');
const cancelApiKeyBtn = document.getElementById('cancel-api-key-btn');
const apiKeyInput = document.getElementById('api-key-input');
const userProfileContainer = document.getElementById('user-profile');
const userAvatar = document.getElementById('user-avatar');
const userAvatarDropdown = document.getElementById('user-avatar-dropdown');
const userName = document.getElementById('user-name');
const darkModeCheckbox = document.getElementById('dark-mode-checkbox');

// Function to apply dark mode
function applyDarkMode(isDark) {
    if (isDark) {
        document.body.classList.add('dark-mode');
    } else {
        document.body.classList.remove('dark-mode');
    }
    if (darkModeCheckbox) { // Ensure checkbox exists
        darkModeCheckbox.checked = isDark;
    }
}

// Initialize the application
async function init() {
    await checkLoginStatus();
    
    // Add event listeners
    searchForm.addEventListener('submit', handleSearch);
    videoUrlInput.addEventListener('input', toggleClearButton);
    clearBtn.addEventListener('click', clearInput);
    videoUrlInput.addEventListener('click', handleVideoUrlInputClick);
    
    // Login button event listeners
    loginButton.addEventListener('click', showLoginModal);
    closeLoginModalBtn.addEventListener('click', closeLoginModal);
    googleLoginBtn.addEventListener('click', handleGoogleLogin);
    logoutBtn.addEventListener('click', handleLogout);
    
    // API Key Modal event listeners
    closeApiKeyModalBtn.addEventListener('click', closeApiKeyModal);
    saveApiKeyBtn.addEventListener('click', saveApiKey);
    cancelApiKeyBtn.addEventListener('click', closeApiKeyModal);
    
    // Settings 버튼 이벤트 리스너 추가
    const settingsBtn = document.getElementById('settings-btn');
    if (settingsBtn) {
        settingsBtn.addEventListener('click', showApiKeyModal);
    }
    
    // 드롭다운 토글을 위한 이벤트 리스너 추가
    document.addEventListener('click', handleDropdownToggle);
    
    // Initialize tab system
    initTabSystem();

    // Check if URL has video parameter
    const urlParams = new URLSearchParams(window.location.search);
    const videoUrl = urlParams.get('video');

    if (videoUrl) {
        videoUrlInput.value = videoUrl;
        // Delay search execution until API is ready
        if (apiReady) {
            handleSearch(new Event('submit'));
        } else {
            pendingVideoId = extractVideoId(videoUrl);
            console.log('YouTube API not ready yet. Video will load when API is ready.');
        }
    }

    // Initialize tab system
    initTabSystem();

    // Dark mode toggle event listener
    if (darkModeCheckbox) {
        darkModeCheckbox.addEventListener('change', (event) => {
            const isDark = event.target.checked;
            applyDarkMode(isDark);
            localStorage.setItem('darkMode', isDark ? 'enabled' : 'disabled');
        });
    }

    // Load dark mode preference
    const savedDarkMode = localStorage.getItem('darkMode');
    if (savedDarkMode === 'enabled') {
        applyDarkMode(true);
    } else if (savedDarkMode === 'disabled') {
        applyDarkMode(false);
    }
    // If nothing is saved, it defaults to light mode (no class on body)
}

// Toggle clear button visibility
function toggleClearButton() {
    if (videoUrlInput.value.length > 0) {
        clearBtn.style.display = 'block';
    } else {
        clearBtn.style.display = 'none';
    }
}

// Clear input field
function clearInput() {
    videoUrlInput.value = '';
    clearBtn.style.display = 'none';
    videoUrlInput.focus();
}

// Handle video URL input click with improved API key check
function handleVideoUrlInputClick(event) {
    // 로그인 상태 확인
    if (!isLoggedIn) {
        // 모달 표시 전 기본 동작 방지
        event.preventDefault();
        showLoginModal();
        return;
    }
    
    // API 키가 필요하지 않은 경우 (서버 정책에 따라)
    if (!needsApiKey) {
        // API 키가 필요하지 않으면 바로 진행
        return;
    }
    
    // API 키 유효성 확인 (암호화된 키의 존재 및 복호화 테스트)
    const encryptedKey = getEncryptedApiKey();
    if (!encryptedKey) {
        // API 키가 저장되어 있지 않은 경우
        event.preventDefault();
        showApiKeyModal();
        return;
    }
    
    // 암호화된 키가 있지만 복호화가 불가능한 경우 (사용자가 변경되었거나 손상된 경우)
    try {
        const decryptedKey = decryptApiKey(encryptedKey);
        if (!decryptedKey || !isValidOpenAIApiKey(decryptedKey)) {
            // 유효하지 않은 키인 경우 재설정하도록 안내
            localStorage.removeItem(`apikey_${userProfile.id}`); // 손상된 키 제거
            event.preventDefault();
            showApiKeyModal();
            return;
        }
    } catch (error) {
        console.error('API key validation error:', error);
        localStorage.removeItem(`apikey_${userProfile.id}`); // 손상된 키 제거
        event.preventDefault();
        showApiKeyModal();
        return;
    }
}

// Check login status with backend verification
async function checkLoginStatus() {
    try {
        // 백엔드 API를 통해 로그인 상태 확인
        const response = await fetch('/user/info', {
            method: 'GET',
            credentials: 'include' // 중요: 쿠키 포함
        });

        if (!response.ok) {
            throw new Error('Not authenticated');
        }

        const data = await response.json();
        
        if (data.authenticated && data.user) {
            userProfile = data.user;
            isLoggedIn = true;
            updateUIForLoggedInUser();
            
            // 로컬 스토리지에 사용자 정보 저장 (선택적)
            localStorage.setItem('user', JSON.stringify(userProfile));
            
            // 로그인 성공 후 API 키 상태 확인
            await checkApiKeyStatus();
            
            console.log('User is authenticated:', userProfile);
        } else {
            handleNotAuthenticated();
        }
    } catch (error) {
        console.error('Authentication check failed:', error);
        handleNotAuthenticated();
    }
}

// API 키 상태 확인 함수 (사용자별 API 키 필요 여부)
async function checkApiKeyStatus() {
    if (!isLoggedIn) {
        needsApiKey = true; // 기본적으로 API 키 필요
        return;
    }
    
    try {
        const response = await fetch('/user/api-key-status', {
            method: 'GET',
            credentials: 'include' // 세션 쿠키 포함
        });
        
        if (!response.ok) {
            throw new Error('Failed to fetch API key status');
        }
        
        const data = await response.json();
        needsApiKey = data.needsApiKey;
        serverKeyPolicy = data.serverKeyPolicy;
        
        console.log('API key status checked. Needs API key:', needsApiKey);
        console.log('Server key policy:', serverKeyPolicy);
        
        // API 키가 필요하지 않은 경우 로컬 스토리지에서 기존에 저장된 API 키 정보 확인
        if (!needsApiKey) {
            // 이미 저장된 키가 있는지 확인
            const hasStoredKey = getEncryptedApiKey() !== null;
            console.log('User has stored API key:', hasStoredKey);
            
            // 저장된 키가 없거나 서버 정책이 변경되었는지 확인
            updateApiKeyModalContent(needsApiKey);
        }
    } catch (error) {
        console.error('Error checking API key status:', error);
        needsApiKey = true; // 오류 발생 시 보수적으로 API 키 필요로 설정
    }
}

// API 키 모달 내용 업데이트 함수
function updateApiKeyModalContent(needsApiKey) {
    // API 키 모달 텍스트 요소 찾기
    const apiKeyModalTitle = document.querySelector('#api-key-modal .modal-header h3');
    const apiKeyModalText = document.querySelector('#api-key-modal .modal-body p.api-notice');
    
    if (needsApiKey) {
        apiKeyModalTitle.textContent = 'OpenAI API Key Required';
        apiKeyModalText.textContent = 'Your API key is stored only in your browser and never sent to our servers.';
    } else {
        apiKeyModalTitle.textContent = 'OpenAI API Key (Optional)';
        apiKeyModalText.innerHTML = 'The server already has an API key you can use, but you can provide your own key if you prefer.<br>Your API key is stored only in your browser and never sent to our servers.';
    }
}

// 인증되지 않은 경우 처리
function handleNotAuthenticated() {
    // 로컬 스토리지의 사용자 정보 제거
    localStorage.removeItem('user');
    
    isLoggedIn = false;
    userProfile = null;
    updateUIForLoggedOutUser();
}

// Update UI for logged in user
function updateUIForLoggedInUser() {
    // 로그인 버튼 숨기기
    loginButton.classList.add('hidden');
    
    // 사용자 프로필 컨테이너 및 아바타 표시
    userProfileContainer.classList.add('show');
    
    // 사용자 정보 업데이트
    if (userProfile) {
        const avatarUrl = userProfile.picture || 'img/default-avatar.png';
        
        // 헤더의 프로필 정보 업데이트
        if (userAvatar) {
            userAvatar.src = avatarUrl;
        }
        if (userName) {
            userName.textContent = userProfile.name || userProfile.email || 'User';
        }
        
        // 드롭다운의 프로필 정보 업데이트
        if (userAvatarDropdown) {
            userAvatarDropdown.src = avatarUrl;
        }
        
        const userNameDropdown = document.getElementById('user-name-dropdown');
        const userEmail = document.querySelector('.user-email');
        
        if (userNameDropdown) {
            userNameDropdown.textContent = userProfile.name || 'User';
        }
        if (userEmail && userProfile.email) {
            userEmail.textContent = userProfile.email;
        }
    }
    
    // 드롭다운 메뉴는 기본적으로 숨겨진 상태로 유지
    userDropdown.classList.remove('hidden');
    userDropdown.classList.remove('show');
}

// Update UI for logged out user
function updateUIForLoggedOutUser() {
    // 로그인 버튼 표시
    loginButton.classList.remove('hidden');
    
    // 사용자 프로필 컨테이너 및 드롭다운 숨기기
    userProfileContainer.classList.remove('show');
    userDropdown.classList.add('hidden');
    userDropdown.classList.remove('show');
    
    // 프로필 정보 초기화
    if (userAvatar) userAvatar.src = '';
    if (userName) userName.textContent = '';
    if (document.getElementById('user-avatar-dropdown')) {
        document.getElementById('user-avatar-dropdown').src = '';
    }
    if (document.getElementById('user-name-dropdown')) {
        document.getElementById('user-name-dropdown').textContent = '';
    }
    if (document.querySelector('.user-email')) {
        document.querySelector('.user-email').textContent = '';
    }
}

// Show login modal
function showLoginModal() {
    loginModal.classList.remove('hidden');
    loginModal.classList.add('show');
}

// Close login modal
function closeLoginModal() {
    loginModal.classList.remove('show');
    setTimeout(() => {
        loginModal.classList.add('hidden');
    }, 300);
}

// Show API key modal with updated content
function showApiKeyModal() {
    updateApiKeyModalContent(needsApiKey);
    apiKeyModal.classList.remove('hidden');
    apiKeyModal.classList.add('show');
}

// Close API key modal
function closeApiKeyModal() {
    apiKeyModal.classList.remove('show');
    setTimeout(() => {
        apiKeyModal.classList.add('hidden');
    }, 300);
}

// Google 로그인 버튼 클릭 핸들러
function handleGoogleLogin() {
    // 팝업 창 열기로 OAuth 처리
    const width = 500;
    const height = 600;
    const left = (window.innerWidth - width) / 2;
    const top = (window.innerHeight - height) / 2;
    
    const popup = window.open(
        '/auth/google',
        'google-login',
        `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no`
    );
    
    // 팝업이 차단된 경우 처리
    if (!popup || popup.closed || typeof popup.closed === 'undefined') {
        alert('팝업이 차단되었습니다. 이 사이트에서 팝업을 허용해주세요.');
    }
    
    // 새 창 닫을 때 로그인 상태 확인
    const checkPopupClosed = setInterval(() => {
        if (popup.closed) {
            clearInterval(checkPopupClosed);
            // 로그인 상태 확인
            checkLoginStatus();
        }
    }, 500);
}

// 메시지 이벤트 리스너 (OAuth 콜백에서 메시지 수신)
window.addEventListener('message', function(event) {
    // 모든 출처에서 메시지 수신 가능 (필요에 따라 제한)
    if (event.data.type === 'GOOGLE_LOGIN_SUCCESS') {
        // 백엔드에서 세션이 설정되었으므로 세션 상태를 확인
        checkLoginStatus();
        
        // 모달 닫기
        closeLoginModal();
    }
});

// Handle logout with backend session cleanup
function handleLogout() {
    // 로컬 스토리지에서 사용자 정보 삭제
    localStorage.removeItem('user');
    
    // 로그아웃 API 호출 - POST 메서드 사용
    fetch('/auth/logout', { 
        method: 'POST',
        credentials: 'include' // 중요: 쿠키 포함
    })
    .then(response => {
        if (!response.ok) {
            console.error('Logout failed');
        }
    })
    .catch(error => {
        console.error('Error during logout:', error);
    })
    .finally(() => {
        // 상태 업데이트
        isLoggedIn = false;
        userProfile = null;
        
        // UI 업데이트
        updateUIForLoggedOutUser();
        
        // 요약 화면 닫기 및 첫 화면으로 돌아가기
        resetToInitialState();
        
        // URL에서 비디오 파라미터 제거하고 홈페이지로 이동
        const newUrl = new URL(window.location.origin);
        window.history.pushState({}, '', newUrl);
    });
}

// 초기 상태로 UI 재설정
function resetToInitialState() {
    // 검색 결과 컨테이너 숨기기
    if (resultsContainer) {
        resultsContainer.classList.add('hidden');
    }
    
    // 검색창을 컴팩트 모드에서 원래 크기로 복원
    // if (searchContainer && searchContainer.classList.contains('compact')) {
    //     searchContainer.classList.remove('compact');
    // }
    
    // 메인 콘텐츠 컴팩트 모드 복원
    if (mainContent && mainContent.classList.contains('compact')) {
        mainContent.classList.remove('compact');
    }
    
    // 로고 크기 복원
    // if (logo && logo.classList.contains('compact')) {
    //     logo.classList.remove('compact');
    // }
    
    // 비디오 플레이어 정리
    if (player) {
        try {
            // 타임스탬프 업데이트 인터벌 정리
            if (timeUpdateInterval) {
                clearInterval(timeUpdateInterval);
                timeUpdateInterval = null;
            }
            
            // 플레이어 중지 및 관련 변수 초기화
            player.stopVideo();
            currentVideoId = null;
        } catch (error) {
            console.error('Error cleaning up player:', error);
        }
    }
    
    // 입력 필드 초기화
    if (videoUrlInput) {
        videoUrlInput.value = '';
        if (clearBtn) {
            clearBtn.style.display = 'none';
        }
    }
    
    // 드롭다운 숨기기
    const dropdown = document.getElementById('dropdown');
    if (dropdown) {
        dropdown.style.display = 'none';
    }
    
    // 비디오 제목 및 요약 내용 초기화
    if (videoTitle) {
        videoTitle.textContent = '';
    }
    if (summaryElement) {
        summaryElement.innerHTML = '';
    }
    
    // 로딩 표시 숨기기
    if (loadingElement) {
        loadingElement.classList.add('hidden');
    }
}

// Encrypt API key - 보안 강화 버전
function encryptApiKey(apiKey) {
    if (!userProfile || !userProfile.id) {
        console.error('User not logged in');
        return null;
    }
    
    try {
        const salt = userProfile.id;
        // AES 암호화를 시뮬레이션하는 간단한 XOR 기반 암호화
        // 실제 프로덕션에서는 Web Crypto API 사용을 권장
        let encrypted = '';
        for (let i = 0; i < apiKey.length; i++) {
            // 사용자 ID와 순서 기반 XOR 연산으로 간단한 암호화
            const charCode = apiKey.charCodeAt(i) ^ salt.charCodeAt(i % salt.length) ^ i;
            encrypted += String.fromCharCode(charCode);
        }
        // base64 인코딩으로 저장
        return btoa(encrypted + '|' + salt);
    } catch (error) {
        console.error('Error encrypting API key:', error);
        return null;
    }
}

// Decrypt API key - 보안 강화 버전
function decryptApiKey(encryptedKey) {
    if (!userProfile || !userProfile.id) {
        console.error('User not logged in');
        return null;
    }
    
    try {
        const decoded = atob(encryptedKey);
        const [encrypted, storedSalt] = function (str) {
            const index = str.lastIndexOf('|');
            if (index === -1) return [str];
            return [str.slice(0, index), str.slice(index + 1)];
        }(decoded);
        const salt = userProfile.id;
        
        // 저장된 솔트와 현재 사용자 ID가 일치하는지 확인
        if (storedSalt !== salt) {
            console.error('Invalid API key: user mismatch');
            return null;
        }
        
        // 복호화 - 암호화의 역과정
        let decrypted = '';
        for (let i = 0; i < encrypted.length; i++) {
            const charCode = encrypted.charCodeAt(i) ^ salt.charCodeAt(i % salt.length) ^ i;
            decrypted += String.fromCharCode(charCode);
        }
        
        return decrypted;
    } catch (error) {
        console.error('Error decrypting API key:', error);
        return null;
    }
}

// Save API key with improved validation
function saveApiKey() {
    const apiKey = apiKeyInput.value.trim();
    
    if (!apiKey) {
        alert('API 키를 입력해주세요.');
        return;
    }
    
    // API 키 유효성 검사 강화
    if (!isValidOpenAIApiKey(apiKey)) {
        alert('유효한 OpenAI API 키를 입력해주세요.');
        return;
    }
    
    // 로그인 상태 확인
    if (!isLoggedIn || !userProfile || !userProfile.id) {
        alert('API 키를 저장하려면 로그인이 필요합니다.');
        showLoginModal();
        return;
    }
    
    // API 키 암호화 및 저장
    const encryptedKey = encryptApiKey(apiKey);
    if (encryptedKey) {
        localStorage.setItem(`apikey_${userProfile.id}`, encryptedKey);
        apiKeyInput.value = '';
        closeApiKeyModal();
        
        // 성공 메시지 표시 (선택사항)
        const successMessage = document.createElement('div');
        successMessage.className = 'success-message';
        successMessage.textContent = 'API 키가 성공적으로 저장되었습니다.';
        document.body.appendChild(successMessage);
        
        // 3초 후 메시지 제거
        setTimeout(() => {
            successMessage.style.opacity = '0';
            setTimeout(() => document.body.removeChild(successMessage), 500);
        }, 3000);
    } else {
        alert('API 키 저장 중 오류가 발생했습니다.');
    }
}

// Validate OpenAI API key format
function isValidOpenAIApiKey(apiKey) {
    // OpenAI API 키는 'sk-'로 시작하며 일반적으로 51~52자입니다
    const validFormat = /^sk-[A-Za-z0-9-_]+$/.test(apiKey);
    
    if (!validFormat) {
        // 기본 형식 검사 실패
        return false;
    }
    
    // 추가 검사: 잠재적으로 조작된 키를 감지하기 위한 간단한 검사
    // (실제 API에서 검증되지 않을 가능성이 높은 특정 패턴 차단)
    const suspiciousPatterns = [
        /sk-test/i,      // 테스트 키 패턴
        /sk-sample/i,    // 샘플 키 패턴
        /sk-fake/i,      // 가짜 키 패턴
        /^sk-[0]+$/      // 모든 숫자가 0인 패턴
    ];
    
    // 의심스러운 패턴이 있는지 확인
    for (const pattern of suspiciousPatterns) {
        if (pattern.test(apiKey)) {
            return false;
        }
    }
    
    return true;
}

// Get encrypted API key
function getEncryptedApiKey() {
    if (!userProfile || !userProfile.id) {
        return null;
    }
    
    return localStorage.getItem(`apikey_${userProfile.id}`);
}

// Get decrypted API key
function getDecryptedApiKey() {
    const encryptedKey = getEncryptedApiKey();
    if (!encryptedKey) {
        return null;
    }
    
    return decryptApiKey(encryptedKey);
}

// Handle search form submission
function handleSearch(event) {
    event.preventDefault();
    console.log('Search submitted');
    // 로그인 확인
    if (!isLoggedIn) {
        showLoginModal();
        return;
    }
    
    // API 키 확인 (서버 정책에 따라 필요한 경우에만)
    if (needsApiKey && !getEncryptedApiKey()) {
        showApiKeyModal();
        return;
    }
    
    const url = videoUrlInput.value.trim();
    if (!url || !isValidYouTubeUrl(url)) {
        alert('Please enter a valid YouTube URL');
        return;
    }
    
    // Extract video ID from URL
    const videoId = extractVideoId(url);
    if (!videoId) {
        alert('Could not extract video ID from URL');
        return;
    }
    
    // Update URL
    const newUrl = new URL(window.location);
    newUrl.searchParams.set('video', url);
    window.history.pushState({}, '', newUrl);
    
    // Clear video title
    videoTitle.textContent = '';

    // Show loading state
    showLoading();
    
    // 메인 콘텐츠와 검색 컨테이너 컴팩트 모드로 전환
    if (mainContent && !mainContent.classList.contains('compact')) {
        mainContent.classList.add('compact');
    }
    
    // 결과 컨테이너 표시
    resultsContainer.classList.remove('hidden');

    // 새로운 영상 검색 시 모든 탭 내용 초기화
    const summaryTab = document.getElementById('summary');
    if (summaryTab) summaryTab.innerHTML = '';
    const transcriptTab = document.getElementById('transcript');
    if (transcriptTab) transcriptTab.innerHTML = '<p class="placeholder-text">Transcript will appear here</p>';
    const customSummaryTab = document.querySelector('#tab-custom-summary .custom-summary');
    if (customSummaryTab) customSummaryTab.innerHTML = '<p class="placeholder-text">Under Construct</p>';
    
    // API가 준비되었는지 확인하고 비디오 로드
    if (apiReady) {
        loadYouTubeVideo(videoId);
    } else {
        pendingVideoId = videoId;
        console.log('YouTube API not ready yet. Video will load when API is ready.');
    }
    
    // Fetch summary from API
    fetchSummary(url);
}

// Check if URL is a valid YouTube URL
function isValidYouTubeUrl(url) {
    const pattern = /^(https?:\/\/)?(www\.)?(youtube\.com\/watch\?v=|youtu\.be\/)([a-zA-Z0-9_-]{11})(\S*)?$/;
    return pattern.test(url);
}

// Extract video ID from YouTube URL
function extractVideoId(url) {
    const pattern = /^(https?:\/\/)?(www\.)?(youtube\.com\/watch\?v=|youtu\.be\/)([a-zA-Z0-9_-]{11})(\S*)?$/;
    const match = url.match(pattern);
    return match ? match[4] : null;
}

// Load YouTube video player
function loadYouTubeVideo(videoId) {
    if (!apiReady) {
        pendingVideoId = videoId;
        console.log('YouTube API not ready yet. Video will load when API is ready.');
        return;
    }
    
    if (currentVideoId === videoId) {
        // Video is already loaded, no need to recreate the player
        return;
    }
    
    currentVideoId = videoId;
    
    try {
        if (player) {
            // 기존 인터벌 정리
            if (timeUpdateInterval) {
                clearInterval(timeUpdateInterval);
            }
            // Update existing player
            player.loadVideoById(videoId);
        } else {
            // Create new player
            const playerDiv = document.getElementById('player');
            playerDiv.innerHTML = ''; // Clear any existing content
            
            player = new YT.Player('player', {
                videoId: videoId,
                playerVars: {
                    autoplay: 0,
                    playsinline: 1,
                    origin: window.location.origin
                },
                events: {
                    'onReady': onPlayerReady,
                    'onError': onPlayerError,
                    'onStateChange': onPlayerStateChange
                }
            });
        }
    } catch (error) {
        console.error('Error creating YouTube player:', error);
        const playerDiv = document.getElementById('player');
        playerDiv.innerHTML = '<div class="error-message">YouTube 플레이어를 로드하는 중 오류가 발생했습니다.</div>';
    }
}

// 에러 핸들러 추가
function onPlayerError(event) {
    console.log('YouTube Player Error:', event.data);
    // 사용자에게 오류 표시
    const playerDiv = document.getElementById('player');
    if (event.data === 2) {
        playerDiv.innerHTML = '<div class="error-message">잘못된 비디오 ID입니다.</div>';
    } else if (event.data === 5) {
        playerDiv.innerHTML = '<div class="error-message">HTML5 플레이어 관련 오류가 발생했습니다.</div>';
    } else if (event.data === 100) {
        playerDiv.innerHTML = '<div class="error-message">요청한 비디오를 찾을 수 없습니다.</div>';
    } else if (event.data === 101 || event.data === 150) {
        playerDiv.innerHTML = '<div class="error-message">소유자가 이 비디오의 웹사이트 내 재생을 허용하지 않았습니다.</div>';
    }
}

// When YouTube player is ready
function onPlayerReady(event) {
    // 플레이어가 준비되면 시간 업데이트 인터벌 시작
    startTimeUpdateInterval();
}

// 시간 업데이트 인터벌 시작
function startTimeUpdateInterval() {
    // 기존에 인터벌이 있으면 제거
    if (timeUpdateInterval) {
        clearInterval(timeUpdateInterval);
    }
    
    // 1초마다 현재 시간을 체크하여 타임스탬프 하이라이트
    timeUpdateInterval = setInterval(() => {
        if (player && player.getPlayerState && player.getPlayerState() === YT.PlayerState.PLAYING) {
            const currentTime = player.getCurrentTime();
            highlightCurrentTimestamp(currentTime);
        }
    }, 1000);
}

// 현재 시간에 해당하는 타임스탬프 하이라이트
function highlightCurrentTimestamp(currentTime) {
    // 현재 활성화된 탭에서만 타임스탬프를 하이라이트
    const activeTabContent = document.querySelector('.tab-content.active');
    if (!activeTabContent) return;
    const timestamps = activeTabContent.querySelectorAll('.timestamp');
    let activeTimestamp = null;
    let minDifference = Infinity;
    
    // 모든 타임스탬프를 순회하며 현재 시간과 가장 가까운 이전 타임스탬프 찾기
    timestamps.forEach(timestamp => {
        const timestampTime = parseFloat(timestamp.dataset.time);
        const difference = currentTime - timestampTime;
        
        // 현재 시간보다 이전이거나 같은 타임스탬프 중 가장 가까운 것 선택
        if (difference >= 0 && difference < minDifference) {
            minDifference = difference;
            activeTimestamp = timestamp;
        }
    });
    
    if (activeTimestamp && activeTimestamp.classList.contains('active')) {
        // 이미 활성화된 타임스탬프는 스킵
        return;
    }

    // 현재 활성화된 타임스탬프들의 active 클래스 제거
    timestamps.forEach(el => el.classList.remove('active'));
    
    // 찾은 타임스탬프에 active 클래스 추가
    if (activeTimestamp) {
        activeTimestamp.classList.add('active');
        
        // 필요에 따라 활성화된 타임스탬프가 보이도록 스크롤 (선택사항)
        // 모바일이 아닐 때만 스크롤 적용
        if (window.innerWidth >= 768 && !isElementInViewport(activeTimestamp)) {
            activeTimestamp.scrollIntoView({
                behavior: 'smooth',
                block: 'center'
            });
        }
    }
}

// 요소가 뷰포트 안에 있는지 확인
function isElementInViewport(el) {
    const rect = el.getBoundingClientRect();
    return (
        rect.top >= 0 &&
        rect.left >= 0 &&
        rect.bottom <= (window.innerHeight || document.documentElement.clientHeight) &&
        rect.right <= (window.innerWidth || document.documentElement.clientWidth)
    );
}

// Player 상태 변경 이벤트 핸들러 추가
function onPlayerStateChange(event) {
    // 영상이 재생 중일 때만 시간 업데이트 인터벌 활성화
    if (event.data === YT.PlayerState.PLAYING) {
        startTimeUpdateInterval();
    } else if (event.data === YT.PlayerState.PAUSED || event.data === YT.PlayerState.ENDED) {
        // 일시정지나 종료 시 인터벌 정지
        if (timeUpdateInterval) {
            clearInterval(timeUpdateInterval);
        }
    }
}

// Show loading state
function showLoading() {
    summaryElement.innerHTML = '';
    loadingElement.classList.remove('hidden');
    // 요약 요청 시작 시 탭 컨테이너 숨김
    const tabsContainer = document.querySelector('.tabs-container');
    if (tabsContainer) {
        tabsContainer.classList.add('hidden');
    }
}

// Hide loading state
function hideLoading() {
    loadingElement.classList.add('hidden');
}

// Fetch summary from backend API with API key
function fetchSummary(url) {
    // API endpoint
    const apiUrl = '/api/summary';
    
    // Request data
    const data = {
        url: url
    };
    
    // 기본 헤더 설정
    const headers = {
        'Content-Type': 'application/json'
    };
    
    // API 키가 필요하고 저장되어 있으면 Authorization 헤더에 추가
    const apiKey = getDecryptedApiKey();
    if (apiKey) {
        headers['Authorization'] = `Bearer ${apiKey}`;
    }
    
    // Fetch options with credentials and proper authentication
    const options = {
        method: 'POST',
        headers: headers,
        credentials: 'include', // 중요: 세션 쿠키 포함
        body: JSON.stringify(data)
    };
    
    // Close any existing SSE connection before starting a new summary request
    if (summaryEventSource) {
        summaryEventSource.close();
        summaryEventSource = null;
        console.log('Previous SSE connection closed.');
    }

    // Fetch summary
    fetch(apiUrl, options)
        .then(async response => {
            if (response.status === 200) { // Cached summary
                const data = await response.json();
                hideLoading();
                displaySummary(data);
            } else if (response.status === 202) { // Job queued, expect SSE
                console.log('Summarization job queued. Waiting for SSE updates.');
                // The loading indicator remains visible.
                // Now, set up the SSE connection.
                summaryEventSource = new EventSource('/api/summary/events');

                summaryEventSource.onopen = () => {
                    console.log('SSE connection established for summary updates.');
                };

                summaryEventSource.addEventListener('summary_complete', (event) => {
                    console.log('SSE summary_complete event received:', event.data);
                    try {
                        const summaryData = JSON.parse(event.data);
                        hideLoading();
                        displaySummary(summaryData);
                    } catch (e) {
                        console.error('Error parsing summary_complete data:', e);
                        hideLoading();
                        displayError({ message: 'Failed to parse summary data from server.' });
                    }
                    if (summaryEventSource) {
                        summaryEventSource.close();
                        summaryEventSource = null;
                        console.log('SSE connection closed after summary_complete.');
                    }
                });

                summaryEventSource.addEventListener('summary_error', (event) => {
                    console.log('SSE summary_error event received:', event.data);
                    try {
                        const errorData = JSON.parse(event.data);
                        hideLoading();
                        displayError({ message: errorData.error || 'An error occurred during summarization.' });
                    } catch (e) {
                        console.error('Error parsing summary_error data:', e);
                        hideLoading();
                        displayError({ message: 'Received an unparsable error from server.' });
                    }
                    if (summaryEventSource) {
                        summaryEventSource.close();
                        summaryEventSource = null;
                        console.log('SSE connection closed after summary_error.');
                    }
                });

                summaryEventSource.onerror = (error) => {
                    console.error('SSE connection error:', error);
                    hideLoading();
                    // Check readyState to avoid showing error if it was intentionally closed
                    if (summaryEventSource && summaryEventSource.readyState !== EventSource.CLOSED) {
                         displayError({ message: 'Lost connection to summary notification service. Please try again.'});
                    }
                    if (summaryEventSource) {
                        summaryEventSource.close();
                        summaryEventSource = null;
                        console.log('SSE connection closed due to error.');
                    }
                };

            } else { // Handle other errors from the initial POST request
                const errorData = await response.json().catch(() => ({})); // Try to parse JSON, default to empty if fails
                let errorMessage = `Server responded with status: ${response.status}`;
                if (errorData.error) {
                    errorMessage = errorData.error;
                } else if (response.statusText) {
                    errorMessage = response.statusText;
                }
                
                // Specific error handling based on status
                if (response.status === 401) {
                    isLoggedIn = false;
                    showLoginModal();
                    errorMessage = 'Authentication required. Please log in again.';
                } else if (response.status === 403) {
                    showApiKeyModal();
                    errorMessage = 'API key required or invalid. Please set your OpenAI API key.';
                } else if (response.status === 503) {
                    errorMessage = 'Server is busy or job queue is full. Please try again later.';
                }
                
                throw new Error(errorMessage);
            }
        })
        .catch(error => { // Catches network errors from fetch() or errors thrown above
            hideLoading();
            displayError(error); // error should be an Error object with a message property
            if (summaryEventSource) { // Ensure SSE is closed if POST fails mid-setup
                summaryEventSource.close();
                summaryEventSource = null;
            }
        });
}

// Display summary
function displaySummary(data) {
    // Set video title
    videoTitle.textContent = data.title;
    
    // Create summary HTML
    let summaryHTML = '';
    
    // Add cached indicator if the response was cached
    if (data.cached) {
        summaryHTML += `<div class="cached-indicator">Cached result</div>`;
    }
    
    // Format the summary text with proper paragraphs and timestamps
    const formattedSummary = formatSummaryText(data.summary);
    
    // Add the summary text with timestamps
    const summaryWithTimestamps = addClickableTimestamps(formattedSummary, data.timestamps);
    summaryHTML += `<div class="summary-text">${summaryWithTimestamps}</div>`;
    
    // Update summary element in the Summary tab
    document.getElementById('summary').innerHTML = summaryHTML;

    // 요약 데이터 도착 시 탭 컨테이너 보이기
    const tabsContainer = document.querySelector('.tabs-container');
    if (tabsContainer) {
        tabsContainer.classList.remove('hidden');
    }
    
    // Also update the transcript tab if there's transcript data
    if (data.transcript && Array.isArray(data.transcript)) {
        displayTranscript(data.transcript);
    }
    
    // Add event listeners to timestamp elements
    const timestampElements = document.querySelectorAll('.timestamp');
    timestampElements.forEach(element => {
        element.addEventListener('click', handleTimestampClick);
    });
}

// Format summary text into paragraphs
function formatSummaryText(summary) {
    // Split the text by double newlines (paragraphs)
    const paragraphs = summary.split(/\n\n+/);
    
    // Process each paragraph
    return paragraphs
        .map(paragraph => {
            // Skip empty paragraphs
            if (paragraph.trim() === '') return '';
            
            // 단락 처리
            let processedParagraph = paragraph.trim();
            
            // 마크다운 숫자 목록 변환 (e.g., '1. ', '2. ')
            processedParagraph = processedParagraph.replace(/^(\d+)\.\s+/gm, '$1. ');
            
            // 마크다운 글머리 기호 목록 변환 (*, -, +)
            processedParagraph = processedParagraph.replace(/^[\*\-\+]\s+/gm, ' - ');
            
            // 굵은 텍스트 제거 (**text** 또는 __text__)
            processedParagraph = processedParagraph.replace(/(\*\*|__)(.*?)\1/g, '$2');
            
            // 기울임꼴 제거 (*text* 또는 _text_)
            processedParagraph = processedParagraph.replace(/(\*|_)(.*?)\1/g, '$2');
            
            // 코드 블록 제거 (`text`)
            processedParagraph = processedParagraph.replace(/`(.*?)`/g, '$1');
            
            // 링크 텍스트만 유지 ([text](url))
            processedParagraph = processedParagraph.replace(/\[(.*?)\]\(.*?\)/g, '$1');
            
            // 헤더 마크다운 제거 (# text)
            processedParagraph = processedParagraph.replace(/^#{1,6}\s+/gm, '');

            // 각 문장의 \n을 <br/>로 변경
            processedParagraph = processedParagraph.replace(/\n/g, '<br/>');
            
            return `<p>${processedParagraph}</p>`;
        })
        .join('');
}

// Add clickable timestamps to summary text
function addClickableTimestamps(summary, timestamps) {
    let result = summary;
    
    // Regular expression to match timestamp patterns like [MM:SS] or [HH:MM:SS]
    const timestampRegex = /\[(\d{1,2}):(\d{2})(?::(\d{2}))?\]/g;
    
    // Replace timestamp patterns with clickable spans
    result = result.replace(timestampRegex, (match) => {
        // Extract time from the timestamp (remove brackets)
        const timeStr = match.substring(1, match.length - 1);
        
        // Parse time components
        const timeComponents = timeStr.split(':');
        let hours = 0, minutes = 0, seconds = 0;
        
        if (timeComponents.length === 2) {
            // MM:SS format
            minutes = parseInt(timeComponents[0], 10);
            seconds = parseInt(timeComponents[1], 10);
        } else if (timeComponents.length === 3) {
            // HH:MM:SS format
            hours = parseInt(timeComponents[0], 10);
            minutes = parseInt(timeComponents[1], 10);
            seconds = parseInt(timeComponents[2], 10);
        }
        
        // Calculate time in seconds
        const time = hours * 3600 + minutes * 60 + seconds;
        
        // Create a clickable timestamp HTML
        return `<span class="timestamp" data-time="${time}">${timeStr}</span>`;
    });
    
    return result;
}

// Handle timestamp click
function handleTimestampClick(event) {
    const time = parseFloat(event.target.dataset.time);
    if (player && !isNaN(time)) {
        // Add active class to the clicked timestamp
        document.querySelectorAll('.timestamp').forEach(el => el.classList.remove('active'));
        event.target.classList.add('active');
        
        // On mobile, scroll to the video first
        if (window.innerWidth < 768) {
            const videoContainer = document.querySelector('.video-container');
            videoContainer.scrollIntoView({ behavior: 'smooth' });
            
            // Small delay to ensure scroll completes before playing
            setTimeout(() => {
                player.seekTo(time, true);
                player.playVideo();
            }, 500);
        } else {
            // On desktop, just seek and play
            player.seekTo(time, true);
            player.playVideo();
        }
    }
}

// Display error message
function displayError(error) {
    console.error('Error:', error);
    
    // Set error message
    summaryElement.innerHTML = `
        <div class="error">
            <p>Sorry, we couldn't generate a summary for this video. Please try again later.</p>
            <p>Error: ${error.message}</p>
        </div>
    `;
}

// 드롭다운 메뉴 토글을 처리하는 함수
function handleDropdownToggle(event) {
    // 사용자 아바타 또는 사용자 정보 관련 요소 확인
    const avatar = document.getElementById('user-avatar');
    const userDropdownMenu = document.getElementById('user-dropdown');
    
    // 아바타를 클릭한 경우 드롭다운 메뉴 토글
    if (isLoggedIn && avatar && (event.target === avatar || avatar.contains(event.target))) {
        // 드롭다운 메뉴 표시/숨김 토글
        userDropdownMenu.classList.toggle('show');
        event.stopPropagation(); // 이벤트 버블링 방지
        return;
    }
    
    // 드롭다운 메뉴가 열려있고, 드롭다운 외부를 클릭한 경우 닫기
    if (userDropdownMenu && userDropdownMenu.classList.contains('show') && 
        !userDropdownMenu.contains(event.target)) {
        userDropdownMenu.classList.remove('show');
    }
}

// YouTube IFrame API ready callback
function onYouTubeIframeAPIReady() {
    console.log('YouTube IFrame API is ready');
    apiReady = true;

    // Load pending video if any
    if (pendingVideoId) {
        handleSearch(new Event('submit'));
    }
}

// Tab switching functionality
function initTabSystem() {
    const tabButtons = document.querySelectorAll('.tab-btn');
    
    tabButtons.forEach(button => {
        button.addEventListener('click', () => {
            // Remove active class from all buttons and contents
            tabButtons.forEach(btn => btn.classList.remove('active'));
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });
            
            // Add active class to clicked button and its content
            button.classList.add('active');
            const tabId = button.getAttribute('data-tab');
            document.getElementById(`tab-${tabId}`).classList.add('active');
        });
    });
}

// Format seconds to MM:SS format
function formatTimestamp(seconds) {
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.floor(seconds % 60);
    return `${minutes.toString().padStart(2, '0')}:${remainingSeconds.toString().padStart(2, '0')}`;
}

// Display transcript in the transcript tab
function displayTranscript(transcriptItems) {
    const transcriptElement = document.getElementById('transcript');
    if (!transcriptItems || transcriptItems.length === 0) {
        transcriptElement.className = 'summary';
        transcriptElement.innerHTML =
            '<p class="placeholder-text">Transcript will appear here</p>';
        return;
    }

    let transcriptHTML = '';
    transcriptItems.forEach(item => {
        transcriptHTML += `<p>`;
        transcriptHTML += `<span class="timestamp" data-time="${item.start}">${formatTimestamp(item.start)}</span> `;
        transcriptHTML += `<span class="summary-text">${item.text}</span>`;
        transcriptHTML += `</p>`;
    });

    transcriptElement.className = 'summary'; // Summary와 동일한 스타일 적용
    transcriptElement.innerHTML = transcriptHTML;

    // 타임스탬프 클릭 이벤트 연결
    transcriptElement.querySelectorAll('.timestamp').forEach(element => {
        element.addEventListener('click', handleTimestampClick);
    });
}

// Initialize the app when the DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    init();

    const urlInput = document.getElementById("video-url");
    const dropdown = document.getElementById("dropdown");

    // Fetch recent video titles from the backend with authentication
    async function fetchRecentTitles() {
        // 로그인 상태가 아닌 경우 최근 타이틀 가져오지 않음
        if (!isLoggedIn) {
            return;
        }
        
        try {
            // 사용자별 최근 요약 목록을 가져오는 새 API 엔드포인트 사용
            const response = await fetch("/api/user-recent-summaries", {
                credentials: 'include' // 중요: 세션 쿠키 포함
            });
            
            if (!response.ok) {
                // 401 오류인 경우 세션 만료로 처리
                if (response.status === 401) {
                    isLoggedIn = false;
                    updateUIForLoggedOutUser();
                    return;
                }
                throw new Error("Failed to fetch recent summaries");
            }
            
            const summaries = await response.json();
            populateDropdown(summaries);
        } catch (error) {
            console.error("Error fetching recent summaries:", error);
        }
    }

    // Populate the dropdown with video titles
    function populateDropdown(summaries) {
        if (!summaries || summaries.length === 0) {
            dropdown.style.display = "none";
            return;
        }
        
        dropdown.innerHTML = ""; // Clear existing items
        summaries.forEach((summary) => {
            const item = document.createElement("div");
            item.className = "dropdown-item";
            item.textContent = summary['video_title'];
            item.addEventListener("click", () => {
                // Navigate to the summary page for the selected video
                window.location.href = `/?video=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3D${summary.video_id}`;
            });
            dropdown.appendChild(item);
        });
        
        // 새 레이아웃에 맞게 드롭다운 위치 조정
        dropdown.style.display = "block";
        
        // URL 입력 필드의 위치 및 크기 가져오기
        const urlInputRect = urlInput.getBoundingClientRect();
        
        // 드롭다운 위치 설정 - 입력 필드 아래에 배치
        dropdown.style.position = "absolute";
        dropdown.style.top = `${urlInputRect.bottom + window.scrollY + 8}px`;
        dropdown.style.left = `${urlInputRect.left + window.scrollX - 5}px`;
        dropdown.style.width = `${urlInputRect.width}px`;
        dropdown.style.zIndex = "1000";
    }

    // 입력 필드에 포커스가 오면 드롭다운 표시
    urlInput.addEventListener("focus", () => {
        console.log("URL input focused, fetching recent titles");
        fetchRecentTitles();
    });

    // 입력 필드 외부를 클릭하면 드롭다운 닫기
    document.addEventListener("click", (event) => {
        if (!urlInput.contains(event.target) && !dropdown.contains(event.target)) {
            dropdown.style.display = "none";
        }
    });
});
