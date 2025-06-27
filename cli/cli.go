package main

import (
	"bufio"
	"context"
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
}
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start interactive chat",
	Run:   runConcurrentChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

// =====================================
// Concurrent Streaming Implementation
// =====================================

// handleUserInput reads keyboard input continuously in a separate goroutine
// userInput: send-only channel to send user messages
// ctx: context to know when to stop
func handleUserInput(userInput chan<- string, ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	for {

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
		case <-ctx.Done():
			// Context cancelled, stop reading input
			return
		}
	}
}

// handleAIStreaming manages WebSocket streaming in a separate goroutine
// client: WebSocket client for reading AI responses
// aiChunks: send-only channel to forward AI text chunks
// ctx: context to know when to stop
func handleAIStreaming(client *client.Client, aiChunks chan<- string, ctx context.Context) {
	// Note: aiChunks will be closed by main goroutine, not here

	// Start streaming - this will block until done or error
	client.StartStreaming(aiChunks, ctx)

	// If we reach here, StartStreaming returned (error or done signal)
	// The main goroutine will handle cleanup
}

func handleDisplay(userInput <-chan string, aiChunks <-chan string, wsClient *client.Client, cancel context.CancelFunc) {
	// Clean state management without local variables
	for {
		select {
		case input, ok := <-userInput:
			// Check if channel was closed
			if !ok {
				fmt.Println("\n📪 User input channel closed")
				cancel()
				return
			}

			// Handle exit commands
			if input == "exit" || input == "quit" {
				fmt.Println("👋 Goodbye!")
				cancel()
				return
			}

			// ✅ Smart interrupt handling with state awareness
			currentState := wsClient.GetState()
			fmt.Printf("\n🔍 Current state: %s\n", currentState)

			switch currentState {
			case client.StateResponding:
				fmt.Println("🛑 AI is responding, interrupting...")
				wsClient.CancelResponse()
				// ✅ No waiting - send message immediately!

			case client.StateCancelling:
				fmt.Println("⏳ Cancelling in progress, sending message anyway...")
				// ✅ No waiting - let the user send!

			case client.StateIdle:
				fmt.Println("✅ Ready to send message")

			case client.StateResponded:

			}

			// Send message to AI
			fmt.Printf("📤 Sending: %s\n", input)
			if err := wsClient.SendMessageAsync(input); err != nil {
				fmt.Printf("❌ Send error: %v\n", err)
				continue
			}

			// Prepare for new AI response
			fmt.Print("🤖 Assistant: ")

		case chunk, ok := <-aiChunks:
			// Check if channel was closed
			if !ok {
				fmt.Println("\n📪 AI chunks channel closed")
				cancel()
				return
			}

			// ✅ Display chunks if we're in responding or responded state
			switch wsClient.GetState() {
			case client.StateResponding, client.StateResponded:
				fmt.Print(chunk)
				if wsClient.GetState() == client.StateResponded && len(chunk) == 0 {
					//wsClient.SetState(client.StateIdle)
				}
			default:
				// Got chunk but not in active display state - might be leftover
				fmt.Printf("\n🔧 Received chunk in %s state (ignored): %q\n", wsClient.GetState(), chunk)
			}
		}
	}
}

func runConcurrentChat(cmd *cobra.Command, args []string) {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		handleUserInput(userInput, ctx)
	}()
	go func() {
		defer wg.Done()
		handleAIStreaming(wsClient, aiChunks, ctx)
	}()
	go func() {
		defer wg.Done()
		handleDisplay(userInput, aiChunks, wsClient, cancel)
	}()

	// Wait for context to be done (e.g., when user exits)
	<-ctx.Done()

	// Wait for all goroutines to finish
	wg.Wait()

	// Now safely close other channels
	close(userInput)
	close(aiChunks)

	fmt.Println("🔚 Concurrent chat ended")
}
