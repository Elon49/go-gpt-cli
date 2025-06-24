package main

import (
    "bufio"
    "fmt"
    "log"
    "os"
    "strings"

    "go-gpt-cli/internal/config"
    "go-gpt-cli/internal/client"
)

func main() {
    // טוען את כל הקונפיגורציה מ-.env
    cfg, err := config.Load()
    if err != nil {
        log.Fatal("Config error:", err)
    }
    
    // יוצר client עם כל הפרמטרים החדשים
    wsClient := client.New(cfg.OpenAIAPIKey, cfg.WebSocketURL, cfg.Model)
    
    // מתחבר
    if err := wsClient.Connect(); err != nil {
        log.Fatal("Connection error:", err)
    }
    defer wsClient.Close()

     // קלט מהמשתמש
     fmt.Print("💬 Your message: ")
     scanner := bufio.NewScanner(os.Stdin)
     scanner.Scan()
     userMessage := strings.TrimSpace(scanner.Text())
     
    if userMessage == "" {
        log.Fatal("No message provided")
    }
    
    // שלח הודעה דינמית
    if err := wsClient.SendMessage(userMessage); err != nil {
        log.Fatal("Send error:", err)
    }

    // מבקש מOpenAI להתחיל לענות
    if err := wsClient.RequestResponse(); err != nil {
        log.Fatal("Request response error:", err)
    }
    
    // קורא תגובה
    wsClient.ReadResponse()
}