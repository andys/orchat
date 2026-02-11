# OrChat

A simple LLM chat web app powered by [OpenRouter](https://openrouter.ai/), built with Go and [Fiber](https://gofiber.io/).

## Features

- Chat with any LLM available on OpenRouter
- Claude-like UI: sidebar with chat history, main chat area
- Model selection dropdown with all OpenRouter-supported models
- Streaming responses via Server-Sent Events (SSE)
- File-based chat history persistence
- Remembers your preferred model

## Quick Start

```bash
# Set your OpenRouter API key
export OPENROUTER_API_KEY="sk-or-..."

# Build and run
go build -o orchat .
./orchat

# Or just run directly
go run .
```

Then open http://localhost:3000 in your browser.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `OPENROUTER_API_KEY` | Yes | — | Your OpenRouter API key |
| `ORCHAT_DATA_DIR` | No | `data` | Directory for storing chats and preferences |
| `PORT` | No | `3000` | HTTP server port |

## Project Structure

```
orchat/
├── main.go              # Fiber server, routes, OpenRouter integration
├── storage/
│   └── storage.go       # File-based chat & preferences persistence
├── static/
│   └── index.html       # Single-page frontend (HTML/CSS/JS)
├── data/                # Created at runtime
│   ├── chats/           # JSON files per chat
│   └── preferences.json # User preferences
├── go.mod
└── go.sum
```
