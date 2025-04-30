FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

# 작업 디렉토리 설정
WORKDIR /app

# 빌드를 위한 아키텍처 인자 설정
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Go 모듈 파일 복사 및 의존성 다운로드
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# 백엔드 소스 코드 복사
COPY backend/ ./

# 백엔드 빌드
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o youtube-summarizer .

# 실행 단계
FROM --platform=$TARGETPLATFORM alpine:latest

# 필요한 패키지 설치
RUN apk update && apk --no-cache add ca-certificates && apk --no-cache add gcompat python3 py3-pip
RUN pip3 install yt-dlp --break-system-packages
WORKDIR /app

RUN mkdir backend
# 빌드된 실행 파일 복사
COPY --from=builder /app/youtube-summarizer ./backend

# 환경 변수 파일 복사
COPY backend/.env.example ./.env.example

# 프론트엔드 파일 복사
COPY frontend/ ./frontend/
COPY presentation/ ./presentation/

# 포트 노출
EXPOSE 8080

# 애플리케이션 실행
CMD ["sh", "-c", "cd /app/backend && ./youtube-summarizer"]