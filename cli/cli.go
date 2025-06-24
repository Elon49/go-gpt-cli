package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"bufio"

	"github.com/spf13/cobra"
	"go-gpt-cli/internal/config"
	"go-gpt-cli/internal/client"
)

var rootCmd = &cobra.Command{
    Use:   "gpt-cli",
    Short: "CLI tool for ChatGPT",
}
var chatCmd = &cobra.Command{
    Use:   "chat", 
    Short: "Start interactive chat",
    Run:   runInteractiveChat,  
}

func init() {
    rootCmd.AddCommand(chatCmd)
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
    defer wsClient.Close()  // סגירה אוטומטית בסוף
    
    fmt.Println("✅ Connected!")
    fmt.Println("🤖 GPT Interactive Chat")
    fmt.Println("Type 'exit' or 'quit' to end")
    fmt.Println("────────────────────────────")
    
    scanner := bufio.NewScanner(os.Stdin)
    
    for {
        fmt.Print("\n💬 You: ")
        
        // קרא קלט מהמשתמש
        if !scanner.Scan() {
            break  // אם לא הצליח לקרוא (Ctrl+C וכו')
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