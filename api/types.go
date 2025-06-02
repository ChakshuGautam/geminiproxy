// Package api defines the data structures and handlers for the GeminiProxy's HTTP API.
// This API allows for monitoring proxy status, viewing statistics, and managing settings.
package api

import "time"

// --- Data Structures for Dashboard Endpoint (/api/v1/dashboard) ---

// ProxyStatus defines the possible operational statuses of the proxy.
// These values correspond to the `status` field in `HandlerStatusItem`.
type ProxyStatus string

// Constants for the possible values of ProxyStatus.
const (
	StatusOnline   ProxyStatus = "online"   // Proxy is operating normally.
	StatusOffline  ProxyStatus = "offline"  // Proxy is not operational (e.g., not started or encountered a fatal error).
	StatusDegraded ProxyStatus = "degraded" // Proxy is operational but experiencing issues (e.g., some keys failing).
	StatusUnknown  ProxyStatus = "unknown"  // Proxy status cannot be determined.
)

// HandlerStatusItem represents the operational status of a specific component or handler within the proxy.
// For the current proxy, this typically represents the overall status of the main proxy service.
type HandlerStatusItem struct {
	Name          string      `json:"name"`          // Name of the handler or component (e.g., "Gemini API Proxy").
	Status        ProxyStatus `json:"status"`        // Current operational status, using the ProxyStatus enum.
	UptimeSeconds int64       `json:"uptimeSeconds"` // Uptime of this handler/component in seconds.
	// Additional details like version or specific error messages could be added here.
}

// RecentErrorItem represents a single recent error event recorded by the proxy.
// This structure is used to provide details about errors in API responses.
type RecentErrorItem struct {
	Timestamp time.Time `json:"timestamp"` // Timestamp of when the error occurred.
	Message   string    `json:"message"`   // Detailed error message, potentially including type, key, model, and original error.
	Source    string    `json:"source,omitempty"` // Optional: Identifier for the source of the error (e.g., "ProxyTransport", "ProxyErrorHandler").
}

// ModelRequestStats provides aggregated statistics for requests made to a specific model
// (though currently, the proxy aggregates stats globally rather than per-model).
type ModelRequestStats struct {
	TotalRequests           int64   `json:"totalRequests"`           // Total number of requests processed for this model.
	SuccessfulRequests      int64   `json:"successfulRequests"`      // Number of successful (e.g., HTTP 2xx) requests for this model.
	FailedRequests          int64   `json:"failedRequests"`          // Number of failed requests for this model.
	AverageLatencyMs        float64 `json:"averageLatencyMs"`        // Average latency of requests for this model, in milliseconds.
	SuccessRatePercent      float64 `json:"successRatePercent"`      // Success rate, calculated as (SuccessfulRequests / TotalRequests) * 100.
	RequestsPerMinute       float64 `json:"requestsPerMinute"`       // Optional: Current requests per minute for this model (placeholder, not implemented).
	TokensProcessedTotal    int64   `json:"tokensProcessedTotal"`    // Optional: Total tokens processed for this model (placeholder, not implemented).
	TokensPerMinute         float64 `json:"tokensPerMinute"`         // Optional: Tokens processed per minute for this model (placeholder, not implemented).
}

// ProxyStatistics contains aggregated request statistics, potentially broken down by model.
// In the current implementation, Model1Requests represents global proxy statistics.
type ProxyStatistics struct {
	OverallSuccessRatePercent float64           `json:"overallSuccessRatePercent"` // Overall success rate across all models/requests.
	Model1Requests            ModelRequestStats `json:"model1Requests"`            // Statistics for the primary model (currently represents global stats).
	Model2Requests            ModelRequestStats `json:"model2Requests"`            // Placeholder for a second model; can be zeroed out if not used.
}

// ApiKeyPerformanceItem represents performance metrics and status for a single API key.
type ApiKeyPerformanceItem struct {
	KeyAlias             string  `json:"keyAlias"`                       // Masked or user-defined alias for the API key (e.g., "...key1").
	TotalRequests        int64   `json:"totalRequests"`                  // Total requests made using this API key.
	SuccessfulRequests   int64   `json:"successfulRequests"`             // Successful requests with this key.
	FailedRequests       int64   `json:"failedRequests"`                 // Failed requests with this key.
	AverageLatencyMs     float64 `json:"averageLatencyMs"`               // Average latency for requests using this key, in milliseconds.
	SuccessRatePercent   float64 `json:"successRatePercent"`             // Success rate for this key.
	IsEnabled            bool    `json:"isEnabled"`                      // Current status: true if the key is enabled for use, false if disabled.
	RequestsPerMinute    float64 `json:"requestsPerMinute,omitempty"`    // Optional: Current requests per minute for this key (placeholder).
	TokensProcessedTotal int64   `json:"tokensProcessedTotal,omitempty"` // Optional: Total tokens processed with this key (placeholder).
}

// APIKeyPerformance provides lists of top and bottom performing API keys,
// typically based on metrics like success rate or request volume.
type APIKeyPerformance struct {
	TopPerformingKeys    []ApiKeyPerformanceItem `json:"topPerformingKeys"`    // List of top N performing API keys.
	BottomPerformingKeys []ApiKeyPerformanceItem `json:"bottomPerformingKeys"` // List of bottom N performing API keys.
}

// SystemInformation provides basic system-level metrics related to the proxy server process.
type SystemInformation struct {
	CPUUsagePercent    float64 `json:"cpuUsagePercent"`    // Current CPU usage of the proxy process (placeholder, 0 if N/A).
	MemoryUsageMB      float64 `json:"memoryUsageMB"`      // Current memory allocated by the Go process (runtime.MemStats.Alloc), in MB.
	TotalMemoryMB      float64 `json:"totalMemoryMB"`      // Total memory obtained from the OS by the Go process (runtime.MemStats.Sys), in MB.
	ProxyUptimeSeconds int64   `json:"proxyUptimeSeconds"` // Total uptime of the proxy server instance in seconds.
}

// DashboardData is the root object returned by the GET /api/v1/dashboard endpoint.
// It aggregates various status, statistics, and performance metrics of the proxy.
type DashboardData struct {
	LastUpdated       time.Time           `json:"lastUpdated"`       // Timestamp indicating when this data snapshot was generated.
	HandlerStatus     []HandlerStatusItem `json:"handlerStatus"`     // Status of the main proxy handler.
	RecentErrors      []RecentErrorItem   `json:"recentErrors"`      // List of recent errors encountered by the proxy.
	ProxyStatistics   ProxyStatistics     `json:"proxyStatistics"`   // Aggregated request statistics.
	APIKeyPerformance APIKeyPerformance   `json:"apiKeyPerformance"` // Performance metrics for API keys.
	SystemInformation SystemInformation   `json:"systemInformation"` // System-level information.
}


// --- Data Structures for Settings Endpoint (/api/v1/settings) ---

// ApiKeySettingItem represents an API key within the settings context.
// It's used for both retrieving current key settings (GET) and updating them (POST).
type ApiKeySettingItem struct {
	KeyAlias  string `json:"keyAlias"`           // Masked or user-defined alias for the API key (e.g., "...key1"). Used for display.
	Key       string `json:"key,omitempty"`      // The actual (full) API key string. This field is REQUIRED when updating settings (POST) to identify the key. It's optional in GET responses for security.
	IsEnabled bool   `json:"isEnabled"`          // Current status: true if the key is enabled, false if disabled. This field is used to update key status in POST requests.
	// Future per-key settings (e.g., rate limits, specific model access) can be added here.
}

// SettingsData is the root object for the GET and POST /api/v1/settings endpoint.
// It encapsulates all configurable settings of the proxy application.
type SettingsData struct {
	LastUpdated              time.Time           `json:"lastUpdated"`              // Timestamp of when the settings data was last generated or updated.
	UIRefreshRateSeconds     int                 `json:"uiRefreshRateSeconds"`     // Refresh rate for the Terminal User Interface (TUI), in seconds.
	APIKeyDisplayFormat      string              `json:"apiKeyDisplayFormat"`      // Format for displaying API keys in the TUI and API (e.g., "Masked", "Full", "First 8 Chars").
	MaxRecentErrorsDisplayed int                 `json:"maxRecentErrorsDisplayed"` // Maximum number of recent errors to store and display.
	APIKeys                  []ApiKeySettingItem `json:"apiKeys"`                  // List of all API keys with their current settings (primarily enabled/disabled status).
	// Future global settings can be added here.
}

// UpdateSettingsRequest was a placeholder and is not directly used if POST /settings
// uses SettingsData as its request body. If distinct fields are needed for update
// versus get, this struct could be reactivated. For now, SettingsData is used for both.
// type UpdateSettingsRequest struct {
// 	UIRefreshRateSeconds     *int                  `json:"uiRefreshRateSeconds,omitempty"`
// 	APIKeyDisplayFormat      *string               `json:"apiKeyDisplayFormat,omitempty"`
// 	MaxRecentErrorsDisplayed *int                  `json:"maxRecentErrorsDisplayed,omitempty"`
// 	APIKeys                  []ApiKeySettingItem   `json:"apiKeys,omitempty"` // For updating enabled/disabled status mainly
// }
