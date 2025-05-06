// Global variables
let player;
let currentVideoId = null;
let apiReady = false; // YouTube API 준비 상태 추적
let pendingVideoId = null; // API 준비 전에 로드할 비디오 ID 저장
let timeUpdateInterval; // Player 상태를 추적하는 변수 추가
let isLoggedIn = false; // 로그인 상태 추적
let userProfile = null; // 사용자 프로필 정보 저장
let googleAuth = null; // Google 인증 객체

// DOM elements
const searchForm = document.getElementById('search-form');
const videoUrlInput = document.getElementById('video-url');
const clearBtn = document.getElementById('clear-btn');
const searchBtn = document.getElementById('search-btn');
const searchContainer = document.querySelector('.search-container');
const resultsContainer = document.getElementById('results-container');
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
    
    // 드롭다운 토글을 위한 이벤트 리스너 추가
    document.addEventListener('click', handleDropdownToggle);

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

// Handle video URL input click
function handleVideoUrlInputClick(event) {
    // 로그인 상태 확인
    if (!isLoggedIn) {
        // 모달 표시 전 기본 동작 방지
        event.preventDefault();
        showLoginModal();
        return;
    }
    
    // 로그인 상태이지만 API 키가 없는 경우
    if (!getEncryptedApiKey()) {
        // 모달 표시 전 기본 동작 방지
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
            
            // API 키 설정 여부 확인
            if (!getEncryptedApiKey()) {
                console.log('No API key set for user, will prompt when needed');
            }

            console.log('User is authenticated:', userProfile);
        } else {
            handleNotAuthenticated();
        }
    } catch (error) {
        console.error('Authentication check failed:', error);
        handleNotAuthenticated();
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

// Show API key modal
function showApiKeyModal() {
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
    });
}

// Encrypt API key
function encryptApiKey(apiKey) {
    // 실제 구현에서는 보안을 위해 적절한 암호화 방식 사용 
    // 이 예시에서는 보안을 위해 사용자 ID를 솔트로 사용한 간단한 인코딩
    if (!userProfile || !userProfile.id) {
        console.error('User not logged in');
        return null;
    }
    
    try {
        const salt = userProfile.id;
        // 실제 애플리케이션에서는 CryptoJS나 WebCrypto API 사용 권장
        const encoded = btoa(apiKey + ':' + salt);
        return encoded;
    } catch (error) {
        console.error('Error encrypting API key:', error);
        return null;
    }
}

// Decrypt API key
function decryptApiKey(encryptedKey) {
    // 실제 구현에서는 보안을 위해 적절한 복호화 방식 사용
    if (!userProfile || !userProfile.id) {
        console.error('User not logged in');
        return null;
    }
    
    try {
        const decoded = atob(encryptedKey);
        const salt = userProfile.id;
        const [apiKey, keySalt] = decoded.split(':');
        
        // 솔트가 일치하는지 확인
        if (keySalt !== salt) {
            console.error('Invalid API key: user mismatch');
            return null;
        }
        
        return apiKey;
    } catch (error) {
        console.error('Error decrypting API key:', error);
        return null;
    }
}

// Save API key
function saveApiKey() {
    const apiKey = apiKeyInput.value.trim();
    
    if (!apiKey) {
        alert('Please enter a valid API key');
        return;
    }
    
    // API 키 유효성 검사 (간단한 형식 체크)
    if (!apiKey.startsWith('sk-')) {
        alert('Please enter a valid OpenAI API key (starts with sk-)');
        return;
    }
    
    // API 키 암호화 및 저장
    const encryptedKey = encryptApiKey(apiKey);
    if (encryptedKey) {
        localStorage.setItem(`apikey_${userProfile.id}`, encryptedKey);
        apiKeyInput.value = '';
        closeApiKeyModal();
    } else {
        alert('Failed to securely store API key');
    }
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
    
    // API 키 확인
    if (!getEncryptedApiKey()) {
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
    
    // If the search container is not in compact mode, make it compact
    if (!searchContainer.classList.contains('compact')) {
        searchContainer.classList.add('compact');
    }
    
    // Show results container
    resultsContainer.classList.remove('hidden');
    
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
    const timestamps = document.querySelectorAll('.timestamp');
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
    
    if (activeTimestamp.classList.contains('active')) {
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
}

// Hide loading state
function hideLoading() {
    loadingElement.classList.add('hidden');
}

// Fetch summary from backend API with API key
function fetchSummary(url) {
    // API endpoint
    const apiUrl = '/api/summary';
    
    // 암호화된 API 키 가져오기
    const apiKey = getDecryptedApiKey();
    if (!apiKey) {
        hideLoading();
        displayError(new Error('API key not found. Please set your OpenAI API key.'));
        return;
    }
    
    // Request data
    const data = {
        url: url
    };
    
    // Fetch options with credentials and proper authentication
    const options = {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${apiKey}`
        },
        credentials: 'include', // 중요: 세션 쿠키 포함
        body: JSON.stringify(data)
    };
    
    // Fetch summary
    fetch(apiUrl, options)
        .then(response => {
            if (!response.ok) {
                // 401 오류인 경우 인증 필요
                if (response.status === 401) {
                    // 세션이 만료된 경우 로그인 다시 요청
                    isLoggedIn = false;
                    showLoginModal();
                    throw new Error('Authentication required. Please log in again.');
                }
                throw new Error('Network response was not ok');
            }
            return response.json();
        })
        .then(data => {
            hideLoading();
            displaySummary(data);
        })
        .catch(error => {
            hideLoading();
            displayError(error);
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
    
    // Update summary element
    summaryElement.innerHTML = summaryHTML;
    
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
        return `<span class="timestamp" data-time="${time}">${match}</span>`;
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
    if (isLoggedIn && avatar && 
       (event.target === avatar || avatar.contains(event.target))) {
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
            const response = await fetch("/api/recent-summaries", {
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
        dropdown.style.display = "block";

        // Adjust dropdown positioning to appear directly below the search input
        const urlInputRect = urlInput.getBoundingClientRect();
        dropdown.style.position = "absolute";
        dropdown.style.top = `${urlInputRect.bottom + window.scrollY + 10}px`;
        dropdown.style.left = `${urlInputRect.left + window.scrollX - 8}px`;
        dropdown.style.width = `${urlInputRect.width}px`;
        dropdown.style.zIndex = "1000";
    }

    // Show dropdown on input focus
    urlInput.addEventListener("focus", fetchRecentTitles);

    // Hide dropdown when clicking outside
    document.addEventListener("click", (event) => {
        if (!urlInput.contains(event.target) && !dropdown.contains(event.target)) {
            dropdown.style.display = "none";
        }
    });
});
