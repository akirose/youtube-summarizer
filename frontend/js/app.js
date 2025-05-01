// Global variables
let player;
let currentVideoId = null;
let apiReady = false; // YouTube API 준비 상태 추적
let pendingVideoId = null; // API 준비 전에 로드할 비디오 ID 저장
let timeUpdateInterval; // Player 상태를 추적하는 변수 추가

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

// Initialize the application
function init() {
    // Add event listeners
    searchForm.addEventListener('submit', handleSearch);
    videoUrlInput.addEventListener('input', toggleClearButton);
    clearBtn.addEventListener('click', clearInput);
    
    // Check if URL has video parameter
    const urlParams = new URLSearchParams(window.location.search);
    const videoUrl = urlParams.get('video');
    
    if (videoUrl) {
        videoUrlInput.value = videoUrl;
        handleSearch(new Event('submit'));
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

// Handle search form submission
function handleSearch(event) {
    event.preventDefault();
    
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

// Fetch summary from backend API
function fetchSummary(url) {
    // API endpoint
    const apiUrl = '/api/summary';
    
    // Request data
    const data = {
        url: url
    };
    
    // Fetch options
    const options = {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
    };
    
    // Fetch summary
    fetch(apiUrl, options)
        .then(response => {
            if (!response.ok) {
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
    if (!timestamps || timestamps.length === 0) {
        return summary;
    }
    
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

// YouTube IFrame API 준비 완료 핸들러
function onYouTubeIframeAPIReady() {
    console.log('YouTube IFrame API is ready');
    apiReady = true;
    
    // 대기 중인 비디오가 있으면 로드
    if (pendingVideoId) {
        loadYouTubeVideo(pendingVideoId);
    }
}

// Initialize the app when the DOM is loaded
document.addEventListener('DOMContentLoaded', init);
