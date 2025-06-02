package geminiproxy

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"runtime" // For Go process memory statistics
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultKeysFile is the default location for the API keys file
	DefaultKeysFile = "gemini.keys"

	// DefaultPort is the default port for the proxy server
	DefaultPort = 8081

	// GeminiAPIEndpoint is the real Gemini API endpoint
	GeminiAPIEndpoint = "https://generativelanguage.googleapis.com"

	// MaxRecentErrors defines how many recent errors to store
	MaxRecentErrors = 10 // Default capacity for recent errors list.
)

// ErrorDetail stores structured information about an error event within the proxy.
// This is used for more detailed error reporting in the TUI and potentially API.
type ErrorDetail struct {
	Timestamp time.Time `json:"timestamp"`    // Time when the error occurred.
	SourceID  string    `json:"sourceId"`     // Identifier for the source of the error (e.g., "ProxyTransport", "ProxyErrorHandler").
	Model     string    `json:"model"`        // Model being queried if applicable and known, else "Unknown".
	APIKeyID  string    `json:"apiKeyId"`     // Masked API key ID (e.g., "...key1") used for the request, if applicable.
	ErrorType string    `json:"errorType"`    // Type of error (e.g., HTTP status code like "500", "429", or "TransportError").
	Message   string    `json:"message"`      // Brief error message.
}

// APIKeyInfo holds statistics and status for a single API key.
// It includes metrics like request counts, latencies, and whether the key is currently enabled for use.
// Each APIKeyInfo is protected by its own mutex for concurrent updates to its fields.
type APIKeyInfo struct {
	Name                        string     // Masked name for display (e.g., "...key1"), safe for UIs.
	Key                         string     // The actual API key string. Should be kept confidential.
	Enabled                     bool       // True if the key is active and can be used for new requests.
	Requests                    uint64     // Total number of requests attempted with this key.
	Successes                   uint64     // Total number of successful requests (typically HTTP 2xx) with this key.
	Failures                    uint64     // Total number of failed requests (non-2xx or transport errors) with this key.
	TotalLatencyMicroseconds    uint64     // Cumulative sum of latencies for all requests made with this key, in microseconds.
	AverageLatencyMicroseconds  uint64     // Average latency for requests made with this key, in microseconds. Calculated as TotalLatencyMicroseconds / Requests.
	Status                      string     // User-facing status string, e.g., "Active", "Disabled". Directly reflects the Enabled state.
	mu                          sync.Mutex // Mutex to protect concurrent updates to the fields of this specific APIKeyInfo instance.
}

// GlobalProxyStats holds aggregated statistics for the entire proxy server operation.
// This includes overall request counts, active connection tracking, error logging, and average latency.
// Access to its fields is protected by a mutex for safe concurrent updates.
type GlobalProxyStats struct {
	TotalRequests                 uint64     // Grand total of all requests processed by the proxy.
	ActiveConnections             int64         // Current number of client connections actively being handled or proxied. Atomically updated.
	TotalSuccesses                uint64        // Grand total of successful requests (HTTP 2xx) across all keys.
	TotalFailures                 uint64        // Grand total of failed requests (non-2xx or transport errors) across all keys.
	TotalDataSentBytes            uint64        // Placeholder for total data sent by the proxy. Currently not implemented.
	TotalDataReceivedBytes        uint64        // Placeholder for total data received by the proxy. Currently not implemented.
	OverallAverageLatencyMicroseconds uint64        // Average latency calculated across all requests processed by the proxy.
	RecentErrors                  []ErrorDetail // A capped list (size MaxRecentErrors) of the most recent structured error details.
	mu                            sync.Mutex    // Mutex to protect concurrent updates to fields of GlobalProxyStats, especially RecentErrors and latency calculations.
	totalLatencyForAverage        uint64        // Internal sum of all request latencies, used to compute OverallAverageLatencyMicroseconds.
}

// HandlerInfo holds information about the proxy handler itself, including its operational status,
// uptime, and basic Go process memory statistics.
type HandlerInfo struct {
	Status        string    // Current operational status of the proxy (e.g., "Online", "Initializing", "Degraded").
	Uptime        time.Time // Timestamp indicating when the proxy server instance was started. Used by clients to calculate uptime duration.
	MemAllocBytes uint64    // Go process memory: bytes of allocated heap objects, via runtime.MemStats.Alloc.
	MemSysBytes   uint64    // Go process memory: total bytes of memory obtained from the OS by the Go runtime, via runtime.MemStats.Sys.
}

// KeyManager is responsible for managing the pool of APIKeyInfo objects.
// It handles API key loading from a file, selection of keys for outgoing requests (round-robin),
// status toggling (enable/disable) of keys, and providing snapshots of key information for monitoring.
type KeyManager struct {
	keys          []*APIKeyInfo // Slice of all available API keys and their associated info.
	currentKeyIdx int           // Index used for round-robin selection of the next key.
	mu            sync.Mutex    // Protects currentKeyIdx and the keys slice structure, particularly during key selection and status toggling.
}

// NewKeyManager creates a new KeyManager instance.
// It reads API keys from the specified keysFile, initializes an APIKeyInfo struct for each key
// (with stats zeroed and status as "Active" and Enabled=true), and prepares the manager for use.
// Returns an error if the keys file cannot be read or if no valid keys are found.
func NewKeyManager(keysFile string) (*KeyManager, error) {
	rawKeys, err := readRawAPIKeys(keysFile)
	if err != nil {
		return nil, err
	}
	if len(rawKeys) == 0 {
		return nil, fmt.Errorf("no API keys found in %s", keysFile)
	}

	apiKeys := make([]*APIKeyInfo, len(rawKeys))
	for i, keyStr := range rawKeys {
		var keyName string
		if len(keyStr) > 5 {
			keyName = fmt.Sprintf("...%s", keyStr[len(keyStr)-5:])
		} else {
			keyName = keyStr
		}
		apiKeys[i] = &APIKeyInfo{
			Name:    keyName,
			Key:     keyStr,
			Enabled: true,        // Initialize all keys as enabled
			Status:  "Active",    // Initial status
		}
	}
	return &KeyManager{keys: apiKeys}, nil
}

// readRawAPIKeys reads plain API key strings from the given filename.
// It processes the file line by line, trimming whitespace.
// Lines starting with '#' (comments) and empty lines are skipped.
// Returns a slice of the raw key strings or an error if the file cannot be read.
func readRawAPIKeys(filename string) ([]string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read keys file '%s': %v", filename, err)
	}

	var rawKeys []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		key := strings.TrimSpace(line)
		// Skip empty lines and comment lines
		if key != "" && !strings.HasPrefix(key, "#") {
			rawKeys = append(rawKeys, key)
		}
	}
	return rawKeys, nil
}

// GetKey selects and returns the next available *enabled* APIKeyInfo from the pool
// using a round-robin strategy. This method is responsible for key rotation.
// It iterates through the keys starting from the current index, skipping any disabled keys.
// If all keys are found to be disabled or if the key pool is empty, it logs a critical error
// and returns nil, indicating no key is available for use.
// This method is thread-safe, protected by the KeyManager's mutex.
func (km *KeyManager) GetKey() *APIKeyInfo {
	km.mu.Lock()
	defer km.mu.Unlock()

	if len(km.keys) == 0 {
		return nil
	}

	for i := 0; i < len(km.keys); i++ {
		idx := (km.currentKeyIdx + i) % len(km.keys)
		keyInfo := km.keys[idx]
		keyInfo.mu.Lock() // Lock while checking Enabled status
		isEnabled := keyInfo.Enabled
		keyInfo.mu.Unlock()

		if isEnabled {
			km.currentKeyIdx = (idx + 1) % len(km.keys) // Next starting point
			log.Printf("Using API key %s", keyInfo.Name)
			return keyInfo
		}
	}

	log.Println("CRITICAL: All API keys are disabled!")
	return nil // No enabled key found
}

// ToggleKeyStatus flips the 'Enabled' status of the API key that matches the provided 'keyStr'.
// It also updates the key's 'Status' string to "Active" or "Disabled" to reflect the new state.
// This operation is thread-safe. The KeyManager's mutex protects the iteration over the keys slice,
// and the individual APIKeyInfo's mutex protects the modification of its 'Enabled' and 'Status' fields.
// Returns an error if the specified key string is not found in the manager.
func (km *KeyManager) ToggleKeyStatus(keyStr string) error {
	km.mu.Lock() // Lock for iterating/finding the key in the main keys slice.
	defer km.mu.Unlock()

	for _, keyInfo := range km.keys {
		if keyInfo.Key == keyStr {
			keyInfo.mu.Lock() // Lock the individual keyInfo for modification.
			keyInfo.Enabled = !keyInfo.Enabled
			if keyInfo.Enabled {
				keyInfo.Status = "Active"
			} else {
				keyInfo.Status = "Disabled"
			}
			log.Printf("Key %s status toggled to: %s by TUI/API", keyInfo.Name, keyInfo.Status) // Added context
			keyInfo.mu.Unlock()
			return nil
		}
	}
	// Attempt to provide a more helpful error message if the key is not found.
	var displayKeyNotFound string
	if len(keyStr) > 5 { // Show last 5 chars if key is long, for partial identification
		displayKeyNotFound = "..." + keyStr[len(keyStr)-5:]
	} else {
		displayKeyNotFound = keyStr // Show full key if it's short
	}
	return fmt.Errorf("key '%s' not found for toggling status", displayKeyNotFound)
}

// SetKeyStatus directly sets the 'Enabled' status of the API key matching the provided 'keyStr'.
// It updates the key's 'Status' string to "Active" or "Disabled" accordingly.
// This method is thread-safe.
// Returns an error if the specified key string is not found.
func (km *KeyManager) SetKeyStatus(keyStr string, enabled bool) error {
	km.mu.Lock() // Lock for iterating/finding the key.
	defer km.mu.Unlock()

	for _, keyInfo := range km.keys {
		if keyInfo.Key == keyStr {
			keyInfo.mu.Lock() // Lock the individual keyInfo for modification.
			keyInfo.Enabled = enabled
			if keyInfo.Enabled {
				keyInfo.Status = "Active"
			} else {
				keyInfo.Status = "Disabled"
			}
			log.Printf("Key %s status explicitly set to: %s (Enabled: %t)", keyInfo.Name, keyInfo.Status, keyInfo.Enabled)
			keyInfo.mu.Unlock()
			return nil
		}
	}
	// Attempt to provide a more helpful error message if the key is not found.
	var displayKeyNotFound string
	if len(keyStr) > 5 {
		displayKeyNotFound = "..." + keyStr[len(keyStr)-5:]
	} else {
		displayKeyNotFound = keyStr
	}
	return fmt.Errorf("key '%s' not found for setting status", displayKeyNotFound)
}


// RecordRequest updates the request count, success/failure counts, and latency metrics for this specific APIKeyInfo.
// - success: A boolean indicating if the request made with this key was successful (typically HTTP 200-299).
// - latency: The duration of the request.
// - reqSize, respSize: Placeholders for request/response sizes in bytes (currently not used for detailed metrics, passed as 0).
// This method is thread-safe, protected by the APIKeyInfo's own mutex.
func (ki *APIKeyInfo) RecordRequest(success bool, latency time.Duration, reqSize int64, respSize int64) {
	ki.mu.Lock()
	defer ki.mu.Unlock()

	ki.Requests++
	if success {
		ki.Successes++
	} else {
		ki.Failures++
	}
	latencyMicros := uint64(latency.Microseconds())
	ki.TotalLatencyMicroseconds += latencyMicros
	if ki.Requests > 0 {
		ki.AverageLatencyMicroseconds = ki.TotalLatencyMicroseconds / ki.Requests
	}
}

// GetKeyInfoSnapshot creates and returns a slice of deep copies of all APIKeyInfo objects managed by KeyManager.
// This snapshot is intended for safe, concurrent access by monitoring tools like the TUI,
// preventing race conditions when reading key statistics and statuses.
// Each APIKeyInfo within the snapshot is also a copy, ensuring data integrity.
// This method is thread-safe, protected by the KeyManager's mutex for iterating its keys,
// and each key's own mutex for reading its data consistently.
func (km *KeyManager) GetKeyInfoSnapshot() []APIKeyInfo {
	km.mu.Lock()
	defer km.mu.Unlock()

	infos := make([]APIKeyInfo, len(km.keys))
	for i, keyInfo := range km.keys {
		keyInfo.mu.Lock() // Lock individual key for consistent read
		infos[i] = APIKeyInfo{ // Create a copy
			Name:                       keyInfo.Name,
			Key:                        keyInfo.Key, // Include key for identification in settings
			Enabled:                    keyInfo.Enabled,
			Requests:                   keyInfo.Requests,
			Successes:                  keyInfo.Successes,
			Failures:                   keyInfo.Failures,
			AverageLatencyMicroseconds: keyInfo.AverageLatencyMicroseconds,
			Status:                     keyInfo.Status, // This status is now updated by ToggleKeyStatus
		}
		keyInfo.mu.Unlock()
	}
	return infos
}

// ProxyServer encapsulates the reverse proxy logic, key management, and statistics collection.
// It uses httputil.NewSingleHostReverseProxy to forward requests to the GeminiAPIEndpoint.
type ProxyServer struct {
	keyManager  *KeyManager       // Manages API keys and their usage.
	server      *http.Server      // The underlying HTTP server instance.
	stats       *GlobalProxyStats // Holds aggregated statistics for the proxy.
	handlerInfo *HandlerInfo      // Holds information about the handler itself (uptime, status).
}

// NewProxyServer creates and configures a new ProxyServer instance.
// - keyManager: An initialized KeyManager with API keys.
// - port: The port on which the proxy server will listen.
// It sets up the reverse proxy with a custom director to inject API keys and a custom transport
// to collect detailed statistics.
func NewProxyServer(keyManager *KeyManager, port int) *ProxyServer {
	targetURL, _ := url.Parse(GeminiAPIEndpoint) // URL of the actual Gemini API.
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Create ProxyServer instance to be used in closures (Director, ErrorHandler, Transport).
	ps := &ProxyServer{
		keyManager: keyManager,
		stats: &GlobalProxyStats{
			RecentErrors: make([]ErrorDetail, 0, MaxRecentErrors), // Initialize with capacity for ErrorDetail.
		},
		handlerInfo: &HandlerInfo{
			Status: "Initializing", // Initial status.
			Uptime: time.Now(),     // Record start time for uptime calculation.
		},
	}

	// Custom Director: Modifies the request before it's sent to the target.
	// This is where API key rotation and injection happen.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req) // Perform default director actions (setting scheme, host).

		// Remove Authorization header if present (e.g., if client sends it for other services).
		req.Header.Del("Authorization")

		apiKeyInfo := ps.keyManager.GetKey() // Get the next available enabled API key.

		if apiKeyInfo == nil {
			log.Printf("ERROR: No enabled API key available for request to %s. Request will proceed without an API key.", req.URL.Path)
			// The request will likely fail at the target API without a key.
			// This failure will be captured by the customTransport and recorded.
		} else {
			// Add the selected API key to the request query parameters.
			q := req.URL.Query()
			q.Set("key", apiKeyInfo.Key)
			req.URL.RawQuery = q.Encode()
		}
		// req.Host is automatically set by the originalDirector to targetURL.Host.
	}

	// Custom ModifyResponse: Allows modification of the response from the target before sending to client.
	// Not extensively used here; most metric collection is in customTransport.
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Example: Add a custom header to the response.
		// resp.Header.Set("X-Proxy-Processed", "true")
		return nil
	}

	// Custom ErrorHandler: Handles errors that occur during the reverse proxy operation
	// (e.g., connection errors to the target server).
    proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
        log.Printf("Reverse proxy error serving %s %s: %v", req.Method, req.URL.String(), err)

        ps.stats.mu.Lock()
        ps.stats.TotalFailures++

        errorDetail := ErrorDetail{
            Timestamp: time.Now(),
            SourceID:  "ProxyErrorHandler", // Error from the reverse proxy itself
            Model:     "Unknown",           // Model is not easily known at this stage
            APIKeyID:  "N/A",               // API key might not have been attached or identified yet
            ErrorType: "ProxyError",        // General proxy error type
            Message:   err.Error(),
        }
        // Prepend error to keep most recent ones at the top
        ps.stats.RecentErrors = append([]ErrorDetail{errorDetail}, ps.stats.RecentErrors...)
        if len(ps.stats.RecentErrors) > MaxRecentErrors {
            ps.stats.RecentErrors = ps.stats.RecentErrors[:MaxRecentErrors] // Cap the list
        }
        ps.stats.mu.Unlock()

		// Note: It's hard to attribute this error to a specific API key here,
		// as the error might occur before a key is even selected or if GetKey() returned nil.
		// The customTransport is better suited for key-specific error attribution if the request reaches that stage.

        http.Error(rw, "Proxy encountered an error", http.StatusBadGateway)
    }


	// Custom Transport (RoundTripper): Wraps the default HTTP transport to intercept
	// the request-response lifecycle for detailed statistics collection.
	baseTransport := http.DefaultTransport // Use a new instance or http.DefaultTransport if specific configs are needed
	proxy.Transport = &customTransport{
		Transport: baseTransport,
		ps:        ps, // Provide reference to ProxyServer for accessing stats and keyManager.
	}

	// Setup HTTP server mux and the server itself.
	mux := http.NewServeMux()
	mux.Handle("/", proxy) // All paths are handled by the reverse proxy.

	ps.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	ps.handlerInfo.Status = "Online" // Mark as online once setup is complete.
	return ps
}


// customTransport is an http.RoundTripper that wraps another RoundTripper (typically http.DefaultTransport)
// to collect metrics about requests and responses.
type customTransport struct {
	Transport http.RoundTripper // The underlying transport to make actual HTTP requests.
	ps        *ProxyServer      // Reference to the parent ProxyServer for stats and key management access.
}

// RoundTrip executes a single HTTP transaction, obtaining the Response for the given Request.
// It's the core of the customTransport, where metrics are recorded before and after the actual request.
func (ct *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Increment active connections counter before the request and decrement after.
	atomic.AddInt64(&ct.ps.stats.ActiveConnections, 1)
	defer atomic.AddInt64(&ct.ps.stats.ActiveConnections, -1)

	// Retrieve the APIKeyInfo associated with this request (if a key was successfully attached in Director).
	// The API key string is in the request URL's query parameters.
	var currentKeyInfo *APIKeyInfo
	requestKeyStr := req.URL.Query().Get("key")

	if requestKeyStr != "" { // Only search for key info if a key string is present.
		// This lookup iterates through the keys. For a very large number of keys,
		// a map-based lookup in KeyManager might be more performant.
		// km.mu protects km.keys and km.currentKeyIdx, not individual keyInfo reads if they are static.
		// However, if key statuses could change elsewhere, locking km or individual keyInfo might be needed.
		// For simplicity, assuming km.keys content (pointers) is static post-init.
		// Individual keyInfo stats are protected by their own mutexes.
		ct.ps.keyManager.mu.Lock() // Lock KeyManager during access to its keys slice to be safe if keys could be reloaded.
		for _, ki := range ct.ps.keyManager.keys {
			if ki.Key == requestKeyStr {
				currentKeyInfo = ki
				break
			}
		}
		ct.ps.keyManager.mu.Unlock()
	}


	startTime := time.Now() // Record start time to calculate latency.
	resp, err := ct.Transport.RoundTrip(req) // Make the actual HTTP request.
	latency := time.Since(startTime) // Calculate total latency for the request.
	latencyMicros := uint64(latency.Microseconds())

	// --- Update Global Proxy Statistics ---
	ct.ps.stats.mu.Lock() // Lock global stats for modification.
	ct.ps.stats.TotalRequests++
	ct.ps.stats.totalLatencyForAverage += latencyMicros
	if ct.ps.stats.TotalRequests > 0 {
		ct.ps.stats.OverallAverageLatencyMicroseconds = ct.ps.stats.totalLatencyForAverage / ct.ps.stats.TotalRequests
	}

	if err != nil { // If an error occurred during transport (e.g., connection refused).
		ct.ps.stats.TotalFailures++
		apiKeyID := "N/A"
		if currentKeyInfo != nil {
			apiKeyID = currentKeyInfo.Name // Use masked name
		}
		errorDetail := ErrorDetail{
			Timestamp: time.Now(),
			SourceID:  "ProxyTransport",
			Model:     extractModelFromPath(req.URL.Path), // Helper to get model from path
			APIKeyID:  apiKeyID,
			ErrorType: "TransportError",
			Message:   err.Error(),
		}
		ct.ps.stats.RecentErrors = append([]ErrorDetail{errorDetail}, ct.ps.stats.RecentErrors...)
		if len(ct.ps.stats.RecentErrors) > MaxRecentErrors {
			ct.ps.stats.RecentErrors = ct.ps.stats.RecentErrors[:MaxRecentErrors]
		}
	} else { // If request was completed, check response status code.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			ct.ps.stats.TotalSuccesses++
		} else {
			ct.ps.stats.TotalFailures++
			apiKeyID := "N/A"
			if currentKeyInfo != nil {
				apiKeyID = currentKeyInfo.Name
			}
			errorDetail := ErrorDetail{
				Timestamp: time.Now(),
				SourceID:  "ProxyTransport",
				Model:     extractModelFromPath(req.URL.Path),
				APIKeyID:  apiKeyID,
				ErrorType: fmt.Sprintf("HTTP %d", resp.StatusCode),
				Message:   http.StatusText(resp.StatusCode), // Or a snippet from response body if available
			}
			ct.ps.stats.RecentErrors = append([]ErrorDetail{errorDetail}, ct.ps.stats.RecentErrors...)
			if len(ct.ps.stats.RecentErrors) > MaxRecentErrors {
				ct.ps.stats.RecentErrors = ct.ps.stats.RecentErrors[:MaxRecentErrors]
			}
		}
		// Future enhancement: Capture actual data sent/received if possible and not too complex.
		// ct.ps.stats.TotalDataSentBytes += uint64(req.ContentLength) (often -1 or 0 for streamed bodies)
		// ct.ps.stats.TotalDataReceivedBytes += uint64(resp.ContentLength) (also often -1)
	}
	ct.ps.stats.mu.Unlock()

	// --- Update API Key Specific Statistics ---
	if currentKeyInfo != nil { // Only update if a key was associated with this request.
		isSuccess := (err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300)
		// reqSize and respSize are hard to get accurately without reading bodies, pass 0 for now.
		currentKeyInfo.RecordRequest(isSuccess, latency, 0, 0)
	}

	return resp, err // Return the original response and error.
}

// extractModelFromPath is a helper function to attempt to parse the model name
// from the request path. This is a simplified example.
// Example path: /v1beta/models/gemini-pro:generateContent -> gemini-pro
// Example path: /v1/models/gemini-1.5-flash-latest:generateContent -> gemini-1.5-flash-latest
func extractModelFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "models" && i+1 < len(parts) {
			modelAndAction := strings.Split(parts[i+1], ":")
			if len(modelAndAction) > 0 {
				return modelAndAction[0]
			}
		}
	}
	return "Unknown"
}

// GetGlobalProxyStats returns a deep copy of the current global proxy statistics.
// This is safe for concurrent access (e.g., by the TUI).
func (p *ProxyServer) GetGlobalProxyStats() GlobalProxyStats {
	p.stats.mu.Lock()
	defer p.stats.mu.Unlock()
	// Create a copy to avoid race conditions when TUI reads it
	statsCopy := *p.stats
	// Create a deep copy of the RecentErrors slice.
	statsCopy.RecentErrors = make([]ErrorDetail, len(p.stats.RecentErrors))
	copy(statsCopy.RecentErrors, p.stats.RecentErrors)
	return statsCopy
}

// GetHandlerInfo returns a copy of the current handler information, including uptime and Go process memory stats.
// This is safe for concurrent access.
func (p *ProxyServer) GetHandlerInfo() HandlerInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats) // Reads current Go process memory stats

	return HandlerInfo{
		Status:        p.handlerInfo.Status, // Could be dynamic later based on health checks
		Uptime:        p.handlerInfo.Uptime, // TUI calculates duration from this
		MemAllocBytes: memStats.Alloc,       // Heap objects + recent stack GC + some other
		MemSysBytes:   memStats.Sys,         // Total memory obtained from OS by the Go runtime
	}
}

func (p *ProxyServer) Start() error {
	log.Printf("Starting Gemini API proxy server on %s", p.server.Addr)
	log.Printf("Proxying requests to: %s", GeminiAPIEndpoint)
	if _, err := url.Parse(GeminiAPIEndpoint); err != nil {
		return fmt.Errorf("invalid GeminiAPIEndpoint: %v", err)
	}
	p.handlerInfo.Status = "Online"
	return p.server.ListenAndServe()
}

// ProxyURL returns the listener address of the proxy server, formatted as a URL string.
// Useful for client configuration and informational logs.
func (p *ProxyServer) ProxyURL() string {
	return fmt.Sprintf("http://localhost%s", p.server.Addr)
}
