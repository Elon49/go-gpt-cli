package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
)

// ConversationState represents the current state of the conversation
type ConversationState int32

const (
	StateIdle       ConversationState = iota // 0 - Waiting for input
	StateResponding                          // 1 - AI is currently responding
	StateCancelling                          // 2 - Cancelling current response
	StateResponded                           // 3 - Response completed successfully
)

// String returns human-readable state name
func (s ConversationState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateResponding:
		return "Responding"
	case StateCancelling:
		return "Cancelling"
	case StateResponded:
		return "Responded"
	default:
		return "Unknown"
	}
}

type Client struct {
	conn   *websocket.Conn
	apiKey string
	wsURL  string
	model  string

	// ✅ Pure Mutex state management - clear and simple
	mu    sync.RWMutex      // Protects state field
	state ConversationState // Now using proper type directly!
}

// =====================================
// State Management API - Clean & Simple
// =====================================

// GetState returns current conversation state (thread-safe)
func (c *Client) GetState() ConversationState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state // ✅ No casting needed!
}

// SetState sets conversation state (thread-safe)
func (c *Client) SetState(newState ConversationState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = newState // ✅ Direct assignment!
}

// IsResponding returns true if AI is currently responding
func (c *Client) IsResponding() bool {
	return c.GetState() == StateResponding
}

// CanInterrupt returns true if we can send a new message (interrupt or start new)
func (c *Client) CanInterrupt() bool {
	state := c.GetState()
	return state == StateIdle || state == StateResponding
}

// TryStartResponse attempts to transition from Idle to Responding
// Returns true if successful, false if already responding
func (c *Client) TryStartResponse() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateIdle {
		c.state = StateResponding
		return true
	}
	return false
}

// TryCancel attempts to transition from Responding to Cancelling
// Returns true if successful, false if not responding
func (c *Client) TryCancel() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateResponding {
		c.state = StateCancelling
		return true
	}
	return false
}

// Reset sets state back to Idle (for cleanup/reset)
func (c *Client) Reset() {
	c.SetState(StateIdle)
}

func New(apiKey, wsURL, model string) *Client {
	return &Client{
		apiKey: apiKey,
		wsURL:  wsURL,
		model:  model,
		state:  StateIdle, // ✅ Initialize with proper type
	}
}

// מחזיר: error אם החיבור נכשל, או nil אם הצליח
func (c *Client) Connect() error {
	u, err := url.Parse(c.wsURL)
	if err != nil {
		return fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	q := u.Query()          // מקבל Values struct (map[string][]string)
	q.Set("model", c.model) // מוסיף ?model=gpt-4o-mini-realtime-preview
	u.RawQuery = q.Encode() // הופך את ה-Values חזרה לstring ושם ב-URL

	headers := http.Header{
		"Authorization": []string{"Bearer " + c.apiKey},
		"OpenAI-Beta":   []string{"realtime=v1"},
	}

	fmt.Printf("🔗 Connecting to: %s\n", u.String())

	// Dial() מחזיר 3 ערכים: connection, HTTP response, error
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		// אם יש response, מוסיף את הstatus code לשגיאה
		if resp != nil {
			return fmt.Errorf("WebSocket connection failed: %w (status: %s)", err, resp.Status)
		}
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}

	// שמירת החיבור ב-struct
	c.conn = conn

	fmt.Println("✅ Connected!")
	return nil
}

func (c *Client) Close() error {
	// Close WebSocket connection
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// =====================================
// Concurrent Streaming Implementation
// =====================================

// StartStreaming reads WebSocket messages concurrently and sends chunks to channel
// chunks: send-only channel for streaming text chunks
// done: receive-only channel to signal when to stop
func (c *Client) StartStreaming(chunks chan<- string, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.Reset() // Clean state on shutdown
			return
		default:
			var response map[string]any
			err := c.conn.ReadJSON(&response)
			if err != nil {
				// Try to send error, but don't block if channel is full or ctx is cancelled
				select {
				case chunks <- fmt.Sprintf("\n❌ Connection error: %v\n", err):
					// Error message sent successfully
				case <-ctx.Done():
					// Context cancelled, skip sending error message
				default:
					// Channel full, skip sending error message
				}
				c.Reset() // Reset state on error
				return
			}

			// Process different message types from OpenAI
			if msgType, ok := response["type"].(string); ok {
				switch msgType {
				case "response.created":
					// AI started generating a response
					fmt.Printf("🎯 AI started responding (State: %s)...\n", c.GetState())

				case "response.text.delta":
					// Only process deltas if we're actually responding
					if c.GetState() == StateResponding {
						if delta, ok := response["delta"].(string); ok {
							// Safe send to chunks channel
							select {
							case chunks <- delta:
								// Delta sent successfully
							case <-ctx.Done():
								// Context cancelled, stop streaming
								return
							default:
								// Channel full, skip this delta
							}
						}
					}

				case "response.done":
					// Check the status of the response.done event
					status := "unknown"
					if responseObj, ok := response["response"].(map[string]interface{}); ok {
						if statusVal, ok := responseObj["status"].(string); ok {
							status = statusVal
						}
					}

					// Handle different response completion statuses
					switch status {
					case "completed":
						// Response completed successfully - back to idle
						c.SetState(StateResponded)
						fmt.Printf("✅ AI response completed successfully (State: %s)\n", c.GetState())

					case "cancelled":
						// Response was cancelled - back to idle
						c.SetState(StateIdle)
						fmt.Printf("🛑 Response cancelled via response.done (State: %s)\n", c.GetState())

					default:
						// Unknown status - log the full JSON and reset state safely
						fmt.Printf("⚠️  Unknown response.done status: %s\n", status)
						fmt.Printf("🔍 Full response.done JSON: %+v\n", response)
						fmt.Printf("📍 Current State: %s\n", c.GetState())
						c.SetState(StateResponded) //?
					}

				case "response.cancelled": //?
					// Response was cancelled via separate event - back to idle
					c.SetState(StateResponded)
					fmt.Printf("🛑 Response cancelled via response.cancelled event (State: %s)\n", c.GetState())

				case "error":
					// Log error and reset state
					fmt.Printf("❌ AI Error: %+v\n", response)
					c.Reset()
				}
			}
		}
	}
}

// SendMessageAsync sends message and requests response without waiting
// Used for concurrent mode where StartStreaming() handles the response
// text: user message to send to OpenAI
func (c *Client) SendMessageAsync(text string) error {

	if !c.TryStartResponse() {
		return fmt.Errorf("failed to start response, state is %s", c.GetState())
	}

	// Build user message for OpenAI Realtime API
	message := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": "user",
			"content": []map[string]any{
				{"type": "input_text", "text": text},
			},
		},
	}

	// Send user message to WebSocket
	if err := c.conn.WriteJSON(message); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Request AI response immediately
	responseRequest := map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"modalities": []string{"text"},
		},
	}

	if err := c.conn.WriteJSON(responseRequest); err != nil {
		return fmt.Errorf("failed to request response: %w", err)
	}

	return nil
	// Note: No waiting for response - StartStreaming() goroutine handles it
}

// =====================================
// Response Management
// =====================================

// CancelResponse - מבטל תגובה נוכחית מה-AI
func (c *Client) CancelResponse() error {
	if !c.TryCancel() {
		return fmt.Errorf("failed to cancel response, state is %s", c.GetState())
	}

	fmt.Println("🛑 Cancelling current AI response...")

	message := map[string]any{
		"type": "response.cancel",
	}

	err := c.conn.WriteJSON(message)
	if err != nil {
		return fmt.Errorf("failed to cancel response: %w", err)
	}

	fmt.Println("✅ Cancel request sent to OpenAI")
	return nil
}
