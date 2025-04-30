# YouTube Video Summarizer - Installation Guide

This guide will help you set up and run the YouTube Video Summarizer application on your system.

## Prerequisites

Before you begin, make sure you have the following installed:

1. **Go** (version 1.20 or higher)
   - Download from: `https://golang.org/dl/`
   - Verify installation with: `go version`

2. **API Keys**
   - **OpenAI API Key**
     - Go to [OpenAI API](https://platform.openai.com/api-keys)
     - Create an account if you don't have one
     - Generate a new API key

## Installation Steps

### 1. Clone or Download the Repository

If you have Git installed:

```bash
git clone <repository-url>
cd youtube_summarizer
```

Or download and extract the ZIP file.

### 2. Set Up API Keys

You can set up your API keys using our provided script:

```bash
chmod +x setup_api_keys.sh
./setup_api_keys.sh
```

Follow the prompts to enter your YouTube and OpenAI API keys.

Alternatively, you can manually edit the `.env` file:

```bash
cp backend/.env.example backend/.env
```

Then edit `backend/.env` and add your API keys:

```properties
OPENAI_API_KEY=your_openai_api_key_here
```

### 3. Install Dependencies

Install Go dependencies:

```bash
cd backend
go mod tidy
```

### 4. Run the Application

#### Option 1: Using Make (Recommended)

From the root directory:

```bash
make run
```

#### Option 2: Using the Shell Script

From the root directory:

```bash
chmod +x run.sh
./run.sh
```

#### Option 3: Running Directly

```bash
cd backend
go run main.go
```

The server will start at `http://localhost:8080`

## Usage

1. Open your web browser and navigate to `http://localhost:8080`
2. Enter a YouTube URL in the search box
3. Click the "Summarize" button
4. Wait for the summary to be generated
5. View the video with its AI-generated summary and clickable timestamps

## Troubleshooting

### Missing API Keys

If you see errors related to missing API keys:

1. Ensure you've set up the `.env` file correctly
2. Check that the API keys you provided are valid

### Go Module Issues

If you encounter Go module errors:

```bash
cd backend
go mod tidy
```

### Port Already in Use

If port 8080 is already in use, edit the `PORT` value in the `.env` file:

```properties
PORT=8081
```

## Configuration Options

You can configure the following options in the `.env` file:

- `PORT`: The port the server runs on (default: 8080)
- `CACHE_DIR`: Directory for caching summaries (default: ./cache)
- `DEBUG`: Enable debug mode (default: false)

## Update and Maintenance

To update the application:

1. Pull the latest changes (if using Git)
2. Run `make setup` to ensure dependencies are up to date
3. Restart the application

## Support

If you encounter any issues, please:

1. Check the troubleshooting section above
2. Check the GitHub repository for known issues
3. Submit a new issue if needed

---
