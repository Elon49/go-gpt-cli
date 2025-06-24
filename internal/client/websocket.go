package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	//"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	conn   *websocket.Conn
	apiKey string
	wsURL  string
	model  string
}

func New(apiKey, wsURL, model string) *Client {
	return &Client{
		apiKey: apiKey,
		wsURL:  wsURL,
		model:  model,
		// conn יישאר nil עד שנקרא ל-Connect()
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
	if c.conn != nil { // בדיקה אם יש חיבור פעיל
		return c.conn.Close()
	}
	return nil
}

func (c *Client) SendMessage(text string) error {
    fmt.Printf("📤 Sending: %s\n", text)

	// יצירת הודעה בפורמט שOpenAI מצפה לו
	// זה JSON object שמגדיר conversation item חדש
	message := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": "user",

			// מערך של תוכן - יכול להיות טקסט, שמע, וכו'
			"content": []map[string]any{
				{
					"type": "input_text",
					"text": text,
				},
			},
		},
	}

	// WriteJSON ממיר את ה-map ל-JSON ושולח
	return c.conn.WriteJSON(message)
}

// RequestResponse - מבקש מOpenAI להתחיל לענות
func (c *Client) RequestResponse() error {
	fmt.Println("🎯 Requesting response from OpenAI...")

	message := map[string]any{
		"type": "response.create", // בקשה להתחלת תגובה
	}

	return c.conn.WriteJSON(message)
}

// ReadResponse - method שקורא תגובות מOpenAI
// לא מחזיר כלום, רק מדפיס את התגובות
// עדכון ReadResponse ב-websocket.go
func (c *Client) ReadResponse() {
	fmt.Println("📥 Waiting for response...")

	var fullResponse []string // לאיסוף הטקסט המלא

	for { // לולאה אינסופית במקום 10
		var response map[string]any
		err := c.conn.ReadJSON(&response)
		if err != nil {
			fmt.Printf("❌ Read error: %v\n", err)
			return
		}

		// בדיקה איזה סוג הודעה זה
		if msgType, ok := response["type"].(string); ok {
			switch msgType {
			case "response.audio_transcript.delta":
				// זה חלק מהטקסט!
				if delta, ok := response["delta"].(string); ok {
					fmt.Print(delta) // הדפס מיד בלי \n
					fullResponse = append(fullResponse, delta)
				}
			case "response.done":
				fmt.Println("\n✅ Response completed!")
				fmt.Printf("📝 Full response: %s\n", strings.Join(fullResponse, ""))
				return
			case "response.audio.delta":
				// כאן יכולנו לטפל באודיו
				// if audioData, ok := response["delta"].(string); ok {
				//     playAudio(audioData)
				// }
			case "error":
				fmt.Printf("\n❌ Error: %+v\n", response)
				return
			default:
				// הודעות אחרות - רק לdebug
				//fmt.Printf("🔍 %s\n", msgType)
			}
		}
	}
}
