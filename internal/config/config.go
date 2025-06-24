// internal/config/config.go
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAIAPIKey string
	Debug        bool
	WebSocketURL string
	Model        string
}

func Load() (*Config, error) {
	// חובה לטעון .env - אם נכשל, זו שגיאה קריטית
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is missing in .env file")
	}

	debug := os.Getenv("DEBUG") == "true"
	webSocketURL := os.Getenv("OPENAI_WEBSOCKET_URL")
	model := os.Getenv("MODEL")

	return &Config{
		OpenAIAPIKey: apiKey,
		Debug:        debug,
		WebSocketURL: webSocketURL,
		Model:        model,
	}, nil
}
