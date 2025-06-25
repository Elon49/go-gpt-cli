package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"go-gpt-cli/internal/client"
	"go-gpt-cli/internal/config"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gpt-cli",
	Short: "CLI tool for ChatGPT",
	Run:   testStreaming,
}
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start interactive chat",
	Run:   runInteractiveChat,
}
var concurrentCmd = &cobra.Command{
	Use:   "chat-stream",
	Short: "Start concurrent streaming chat",
	Run:   runConcurrentChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(concurrentCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runInteractiveChat(cmd *cobra.Command, args []string) {
	fmt.Println("🔗 Connecting to OpenAI...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Config error:", err)
	}
	wsClient := client.New(cfg.OpenAIAPIKey, cfg.WebSocketURL, cfg.Model)

	if err := wsClient.Connect(); err != nil {
		log.Fatal("Connection error:", err)
	}
	defer wsClient.Close() // סגירה אוטומטית בסוף

	fmt.Println("✅ Connected!")
	fmt.Println("🤖 GPT Interactive Chat")
	fmt.Println("Type 'exit' or 'quit' to end")
	fmt.Println("────────────────────────────")

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("\n💬 You: ")

		// קרא קלט מהמשתמש
		if !scanner.Scan() {
			break // אם לא הצליח לקרוא (Ctrl+C וכו')
		}

		input := strings.TrimSpace(scanner.Text())

		// דילוג על הודעות ריקות
		if input == "" {
			continue
		}

		// בדיקת יציאה
		if input == "exit" || input == "quit" {
			fmt.Println("👋 Goodbye!")
			break
		}

		sendAndReceiveMessage(wsClient, input)
	}
}

func sendAndReceiveMessage(wsClient *client.Client, message string) {
	// שלח הודעה
	if err := wsClient.SendMessage(message); err != nil {
		log.Printf("❌ Send error: %v", err)
		return
	}

	// בקש תגובה
	if err := wsClient.RequestResponse(); err != nil {
		log.Printf("❌ Request response error: %v", err)
		return
	}

	// קבל תגובה
	fmt.Print("🤖 Assistant: ")
	wsClient.ReadResponse()
}

func testStreaming(cmd *cobra.Command, args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Config error:", err)
	}

	wsClient := client.New(cfg.OpenAIAPIKey, cfg.WebSocketURL, cfg.Model)

	if err := wsClient.Connect(); err != nil {
		log.Fatal("Connection error:", err)
	}
	defer wsClient.Close()

	fmt.Println("🧪 Testing concurrent streaming...")

	// Create channels
	chunks := make(chan string, 10)
	done := make(chan bool)

	// Start streaming in goroutine
	go wsClient.StartStreaming(chunks, done)

	// Send message using OLD functions (they still work!)
	if err := wsClient.SendMessage("hello"); err != nil { // ← ישן
		log.Fatal("Send error:", err)
	}

	if err := wsClient.RequestResponse(); err != nil { // ← ישן
		log.Fatal("Request response error:", err)
	}

	// Read from NEW streaming
	fmt.Print("🤖 Streaming: ")
	for chunk := range chunks {
		fmt.Print(chunk)
	}

	fmt.Println("\n✅ Streaming test completed!")
}

// =====================================
// Concurrent Streaming Implementation
// =====================================

// handleUserInput reads keyboard input continuously in a separate goroutine
// userInput: send-only channel to send user messages
// done: receive-only channel to know when to stop
func handleUserInput(userInput chan<- string, done <-chan bool) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("💬 You: ")

		if !scanner.Scan() {
			return
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		select {
		case userInput <- input:
			// Sent successfully
		case <-done:
			return
		}
	}
}

// handleAIStreaming manages WebSocket streaming in a separate goroutine
// client: WebSocket client for reading AI responses
// aiChunks: send-only channel to forward AI text chunks
// done: receive-only channel to know when to stop
func handleAIStreaming(client *client.Client, aiChunks chan<- string, done <-chan bool) {
	// Note: aiChunks will be closed by main goroutine, not here

	// Start streaming - this will block until done or error
	client.StartStreaming(aiChunks, done)

	// If we reach here, StartStreaming returned (error or done signal)
	// The main goroutine will handle cleanup
}

func handleDisplay(userInput <-chan string, aiChunks <-chan string, wsClient *client.Client, done chan<- bool) {
	// State management for current AI response
	var currentResponse strings.Builder // Accumulate full response text
	var responseActive bool             // Track if AI is currently responding

	for {
		select {
		case input, ok := <-userInput:
			// Check if channel was closed
			if !ok {
				fmt.Println("\n📪 User input channel closed")
				done <- true
				return
			}

			// Handle exit commands
			if input == "exit" || input == "quit" {
				fmt.Println("👋 Goodbye!")
				done <- true
				return
			}

			// Send message to AI asynchronously
			fmt.Printf("📤 Sending: %s\n", input)
			if err := wsClient.SendMessageAsync(input); err != nil {
				fmt.Printf("❌ Send error: %v\n", err)
				continue // Skip this message, keep listening
			}

			// Prepare for new AI response
			currentResponse.Reset()    // Clear previous response
			responseActive = true      // Mark that AI is responding
			fmt.Print("🤖 Assistant: ") // Start the response line

		case chunk, ok := <-aiChunks:
			// Check if channel was closed
			if !ok {
				fmt.Println("\n📪 AI chunks channel closed")
				done <- true
				return
			}

			// Handle AI response chunks
			if responseActive {
				// Check for error messages
				if strings.Contains(chunk, "❌") {
					fmt.Print(chunk)
					responseActive = false
					continue
				}

				// Print chunk immediately (real-time streaming!)
				fmt.Print(chunk)

				// Also store in builder for full response tracking
				currentResponse.WriteString(chunk)

				// Check if this chunk indicates response is done
				// (Based on newline which indicates StartStreaming sent completion)
				if strings.Contains(chunk, "\n") && !strings.Contains(chunk, "❌") {
					// Response completed successfully
					responseActive = false
					fmt.Printf("✅ Response completed!\n")
					// Optionally show full response length
					fullText := strings.TrimSpace(currentResponse.String())
					if len(fullText) > 0 {
						fmt.Printf("📊 Response length: %d characters\n", len(fullText))
					}
				}
			}
		}
	}
}

func runConcurrentChat(cmd *cobra.Command, args []string) {
	// Setup
	fmt.Println("🔗 Connecting to OpenAI (Concurrent mode)...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Config error:", err)
	}

	wsClient := client.New(cfg.OpenAIAPIKey, cfg.WebSocketURL, cfg.Model)

	if err := wsClient.Connect(); err != nil {
		log.Fatal("Connection error:", err)
	}
	defer wsClient.Close()

	// Create channels for communication between goroutines
	userInput := make(chan string, 5) // Buffer for user messages
	aiChunks := make(chan string, 50) // Buffer for AI response chunks
	done := make(chan bool, 1)        // Signal channel for shutdown (buffered to prevent blocking)

	fmt.Println("🚀 Starting concurrent chat...")
	fmt.Println("🤖 GPT Concurrent Chat")
	fmt.Println("Type 'exit' or 'quit' to end")
	fmt.Println("────────────────────────────")

	// WaitGroup to ensure all goroutines finish before cleanup
	var wg sync.WaitGroup

	// Start 3 goroutines
	wg.Add(3)
	go func() {
		defer wg.Done()
		handleUserInput(userInput, done)
	}()
	go func() {
		defer wg.Done()
		handleAIStreaming(wsClient, aiChunks, done)
	}()
	go func() {
		defer wg.Done()
		handleDisplay(userInput, aiChunks, wsClient, done)
	}()

	// Wait for completion (blocks until someone sends to done channel)
	<-done

	// Close the done channel to signal all goroutines to stop
	close(done)

	// Wait for all goroutines to finish
	wg.Wait()

	// Now safely close other channels
	close(userInput)
	close(aiChunks)

	fmt.Println("🔚 Concurrent chat ended")
}
