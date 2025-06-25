# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building and Running
```bash
# Build the application
go build -o gpt-cli

# Run directly with Go
go run main.go                    # Default command (test streaming)
go run main.go chat              # Interactive chat mode
go run main.go chat-stream       # Concurrent streaming chat mode

# Run built binary
./gpt-cli                        # Default test streaming
./gpt-cli chat                   # Interactive chat
./gpt-cli chat-stream            # Concurrent streaming
```

### Testing and Linting
```bash
# Run tests
go test ./...

# Format code
go fmt ./...

# Vet code for common issues
go vet ./...

# Tidy dependencies
go mod tidy
```

## Architecture Overview

This is a Go CLI application that provides an interactive chat interface with OpenAI's GPT models using WebSocket real-time streaming. The application supports both synchronous and concurrent chat modes.

### Core Architecture

**Package Structure:**
- `main.go` - Entry point with basic streaming test
- `cli/cli.go` - Cobra CLI commands and chat implementations
- `internal/client/websocket.go` - WebSocket client for OpenAI Realtime API
- `internal/config/config.go` - Configuration management with .env support

**Key Dependencies:**
- `github.com/spf13/cobra` - CLI framework
- `github.com/gorilla/websocket` - WebSocket client
- `github.com/joho/godotenv` - Environment variable loading

### Configuration Requirements

The application requires a `.env` file with:
```env
OPENAI_API_KEY=your-api-key-here
OPENAI_WEBSOCKET_URL=wss://api.openai.com/v1/realtime
MODEL=gpt-4o-mini-realtime-preview
DEBUG=false
```

### Chat Modes

**Interactive Chat (`chat` command):**
- Sequential request/response pattern
- Simple user input loop with exit commands (`exit`/`quit`)
- Synchronous WebSocket communication

**Concurrent Streaming (`chat-stream` command):**
- Real-time streaming with concurrent goroutines
- Three goroutines: user input, AI streaming, display coordination
- Uses channels for inter-goroutine communication
- Proper goroutine lifecycle management with sync.WaitGroup

### WebSocket Client Architecture

The client supports both synchronous and asynchronous patterns:

**Synchronous Methods:**
- `SendMessage(text)` - Send user message
- `RequestResponse()` - Request AI response
- `ReadResponse()` - Block and read complete response

**Asynchronous Methods:**
- `SendMessageAsync(text)` - Non-blocking message send
- `StartStreaming(chunks, done)` - Concurrent response streaming

**Key Features:**
- Automatic reconnection on connection failures
- Proper error handling and recovery
- Channel-based concurrent communication
- Safe goroutine shutdown patterns

### Concurrency Implementation

The concurrent chat mode demonstrates advanced Go patterns:
- **Channel Communication:** Typed channels for safe data passing
- **Goroutine Coordination:** WaitGroup for proper lifecycle management
- **Select Statements:** Non-blocking channel operations
- **Safe Shutdown:** Proper channel closing and goroutine termination

See `docs/NOTES.md` for detailed concurrency implementation notes and technical documentation.

## Development Notes

- The codebase includes Hebrew comments in some files - this is expected
- Error messages use emoji prefixes for visual clarity (🔗, ✅, ❌, etc.)
- The WebSocket implementation handles OpenAI Realtime API message formats
- Proper error handling includes reconnection logic for dropped connections
- Channel buffer sizes are optimized for different data types (5 for user input, 50 for AI chunks)