# YouTube Video Summarizer

A web application that summarizes YouTube videos and allows users to navigate to specific timestamps. The application has a clean, Google-like interface with both frontend and backend components, and uses OpenAI's GPT-4o-mini model for generating summaries.

## Features

- Input YouTube URLs to get AI-generated summaries
- View summaries with clickable timestamps
- Navigate directly to specific parts of the video
- Responsive design that works on desktop and mobile
- Clean, Google-inspired interface

## Tech Stack

- **Frontend**: HTML, CSS, JavaScript
- **Backend**: Golang with GIN framework
- **APIs**: YouTube Data API, OpenAI API (GPT-4o-mini)

## Project Structure

```
youtube_summarizer/
├── frontend/           # Frontend code
│   ├── css/            # Stylesheets
│   ├── js/             # JavaScript files
│   └── index.html      # Main HTML file
└── backend/            # Backend code
    ├── main.go         # Entry point
    ├── api/            # API endpoints
    ├── services/       # Business logic
    └── models/         # Data models
```

## Setup and Installation

### Prerequisites

- Go 1.20 or higher
- OpenAI API key

### Backend Setup

1. Clone this repository
2. Navigate to the `backend` directory
3. Copy `.env.example` to `.env` and fill in your API keys
4. Install dependencies:

   ```bash
   go mod tidy
   ```

5. Run the server using one of these methods:

   **Using Make** (recommended):

   ```bash
   make run
   ```

   **Using the shell script**:

   ```bash
   chmod +x run.sh
   ./run.sh
   ```

   **Running directly**:

   ```bash
   cd backend
   go run main.go
   ```

The server will start at `http://localhost:8080`

### Using the Makefile

The project includes a Makefile for easy management:

```bash
# Setup the project (install dependencies, create .env file)
make setup

# Run the application
make run

# Build the application
make build

# Run tests
make test

# Clean build artifacts
make clean

# Show help message
make help
```

## API Endpoints

- `POST /api/summary`: Submit a YouTube URL for summarization
  - Request: `{ "url": "https://www.youtube.com/watch?v=..." }`
  - Response: `{ "videoId": "...", "title": "...", "summary": "...", "timestamps": [...] }`

## Usage

1. Open the application in your browser
2. Paste a YouTube URL in the search box
3. Click "Summarize" to generate a summary
4. View the summary with clickable timestamps
5. Click on timestamps to navigate to specific parts of the video

## Limitations

- The application requires the YouTube video to have captions/transcripts available
- Very long videos may result in less detailed summaries due to token limitations
- The quality of the summary depends on the clarity and quality of the video transcript

## Future Improvements

- Add support for multiple languages
- Implement caching for previously summarized videos
- Add more customization options for summaries
- Support for playlists and channels
