package services

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessSubtitleFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create mock subtitle files
	mockVttContent := `WEBVTT
Kind: captions
Language: ko

00:00:00.000 --> 00:00:02.033
AI 를 사용해서 개발할 때 가장

00:00:02.033 --> 00:00:03.133
문제가 되는 부분은

00:00:03.166 --> 00:00:04.666
저희의 코드 베이스가 커지면

00:00:04.666 --> 00:00:05.366
커질수록

00:00:05.366 --> 00:00:07.233
오류가 발생할 확률이 높아지고

00:00:07.400 --> 00:00:07.933
그것들이

00:00:07.933 --> 00:00:09.266
디버깅하기가 굉장히

00:00:09.266 --> 00:00:10.600
어려워진다는 부분인데요

00:00:10.666 --> 00:00:11.533
AI 를 사용하면

00:00:11.533 --> 00:00:13.033
AI 를 사용하지 않을 때 보다

00:00:13.033 --> 00:00:14.866
개발 속도는 굉장히 빨라 지지만

00:00:15.033 --> 00:00:16.300
개발 속도가 빨라진 만큼
`
	mockFilePath := tempDir + "/mock.vtt"
	err := os.WriteFile(mockFilePath, []byte(mockVttContent), 0644)
	assert.NoError(t, err)

	// Call the function
	chunkSize := 10.0
	chunks, err := processSubtitleFiles(tempDir, chunkSize)

	// Assertions
	assert.NoError(t, err)
	assert.Len(t, chunks, 2)
}
