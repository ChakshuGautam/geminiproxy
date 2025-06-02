// cmd/geminiproxy/main.go is the main entry point for the Gemini API Proxy application.
// It initializes the key manager, the proxy server, and the terminal user interface (TUI),
// then starts the proxy server in a background goroutine and runs the TUI in the main goroutine.
package main

import (
	"fmt"      // For formatted string output, e.g., server address.
	"log"      // For logging messages, especially errors.
	"net/http" // For HTTP server, ServeMux, and status codes.

	"geminiproxy"       // Core proxy logic including KeyManager and ProxyServer.
	"geminiproxy/api"   // API handlers and types.
	"geminiproxy/tui"   // Terminal User Interface package.
)

const (
	DefaultAPIPort = "8080" // Default port for the API server.
)

func main() {
	// --- Initialize Core Components ---
	log.Println("Initializing KeyManager...")
	km, err := geminiproxy.NewKeyManager(geminiproxy.DefaultKeysFile)
	if err != nil {
		log.Fatalf("FATAL ERROR: Creating KeyManager failed: %v", err)
	}
	log.Println("KeyManager initialized successfully.")

	log.Println("Initializing ProxyServer...")
	proxy := geminiproxy.NewProxyServer(km, geminiproxy.DefaultPort) // Using DefaultPort from geminiproxy package
	log.Printf("ProxyServer configured. Clients can connect to: %s (Gemini API Proxy)", proxy.ProxyURL())

	log.Println("Initializing SettingsManager for API...")
	settingsManager := api.NewSettingsManager()
	log.Println("SettingsManager initialized.")

	// --- Start Proxy Server ---
	// The ProxyServer handles the actual forwarding of requests to the Gemini API.
	go func() {
		log.Printf("Attempting to start ProxyServer on %s...", proxy.ProxyURL())
		if err := proxy.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("FATAL ERROR: ProxyServer failed to start: %v", err)
		}
		log.Println("ProxyServer has shut down.")
	}()

	// --- Configure and Start API Server ---
	// The API server provides endpoints for dashboard data and settings management.
	apiRouter := http.NewServeMux()
	apiBasePath := "/api/v1" // Base path for all v1 API endpoints.
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = DefaultAPIPort
	}

	// Register Dashboard endpoint handler
	apiRouter.HandleFunc(apiBasePath+"/dashboard", api.DashboardHandler(proxy, km))

	// Register Settings endpoint handlers (GET and POST)
	settingsGetHandler := api.GetSettingsHandler(settingsManager, km)
	settingsUpdateHandler := api.UpdateSettingsHandler(settingsManager, km)
	apiRouter.HandleFunc(apiBasePath+"/settings", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			settingsGetHandler(w, r)
		case http.MethodPost:
			settingsUpdateHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	apiServerAddr := fmt.Sprintf(":%s", apiPort)
	apiHttpServer := &http.Server{
		Addr:    apiServerAddr,
		Handler: apiRouter,
		// Consider adding ReadTimeout, WriteTimeout, IdleTimeout for production robustness.
	}

	go func() {
		log.Printf("API Server starting on http://localhost%s%s", apiServerAddr, apiBasePath)
		if err := apiHttpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("FATAL ERROR: API Server failed: %v", err)
		}
		log.Println("API Server has shut down.")
	}()

	// --- Start Terminal User Interface (TUI) ---
	// The TUI is the primary blocking process for the main goroutine.
	// It provides interactive monitoring and management.
	log.Println("Starting Terminal User Interface (TUI)...")
	tui.Start(proxy, km) // This blocks until TUI exits.

	// --- Application Shutdown ---
	// This log message is printed after the TUI has been exited.
	// Further shutdown logic for API server and Proxy server could be added here if needed
	// (e.g., graceful shutdown with context), but for now, they exit when main exits.
	log.Println("TUI exited. Application is shutting down.")
}
