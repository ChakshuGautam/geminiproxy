package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"geminiproxy"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// These constants are from the geminiproxy package
const (
	DefaultKeysFile = "gemini.keys"
	DefaultPort     = 8081
)

func main() {
	// Create the key manager (reads from gemini.keys by default)
	keyManager, err := geminiproxy.NewKeyManager(geminiproxy.DefaultKeysFile)
	if err != nil {
		log.Fatalf("ERROR: Could not create key manager: %v. Ensure '%s' exists and contains keys.", err, geminiproxy.DefaultKeysFile)
	}

	// Create the proxy server
	proxy := geminiproxy.NewProxyServer(keyManager, geminiproxy.DefaultPort)
	proxyURL := proxy.ProxyURL()
	log.Printf("Gemini API proxy starting at %s", proxyURL)

	// Start the server in a goroutine so the main function can continue
	go func() {
		if err := proxy.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ERROR: Could not start proxy server: %v", err)
		}
	}()

	// Give the server a moment to start up
	time.Sleep(200 * time.Millisecond)

	// Test with go-genai client and demonstrate key rotation
	log.Println("\n--- Testing with go-genai Client (with key rotation) ---")
	testGoGenAIClient(proxyURL)

	// Keep the server running (optional, remove or comment out if not needed)
	log.Println("\nProxy server is running. Press Ctrl+C to stop.")
	select {}
}

// testGoGenAIClient tests the proxy using the official go-genai client
// and demonstrates API key rotation across multiple requests
func testGoGenAIClient(proxyURL string) {
	// Set up the client to use our proxy
	ctx := context.Background()
	client, err := genai.NewClient(ctx,
		option.WithAPIKey("DUMMY_API_KEY"), // Dummy key - our proxy will replace this
		option.WithEndpoint(proxyURL),      // Use our proxy URL
	)
	if err != nil {
		log.Fatalf("Failed to create go-genai client: %v", err)
	}
	defer client.Close()

	// Create a model instance
	model := client.GenerativeModel("gemini-2.0-flash")

	// Different prompts to demonstrate key rotation
	prompts := []string{
		"Write a short poem about Go programming",
		"Write a haiku about API proxies",
		"Tell me a short joke about programming",
	}

	log.Printf("Making %d requests to demonstrate key rotation...", len(prompts))

	// Make multiple requests to demonstrate key rotation
	for i, prompt := range prompts {
		log.Printf("\nRequest %d: %q", i+1, prompt)

		// Generate content
		resp, err := model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			log.Printf("WARN: Request %d failed: %v", i+1, err)
			continue
		}

		// Print the response
		log.Printf("Response %d:", i+1)
		for _, cand := range resp.Candidates {
			for _, part := range cand.Content.Parts {
				log.Printf("Content: %v", part)
			}
		}

		// Small delay between requests
		time.Sleep(500 * time.Millisecond)
	}

	log.Println("\n--- Key Rotation Test Complete ---")
	log.Println("Check the logs above. You should see lines like 'Using API key ending in ...<key_suffix>' for each key in gemini.keys.")
}
