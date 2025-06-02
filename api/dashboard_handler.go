package api

import (
	"encoding/json"
	"geminiproxy"
	"log"
	"net/http"
	"runtime"
	"sort"
	"time"
)

// formatBytesForAPI is a helper function to convert byte counts (uint64) into megabytes (float64)
// specifically for the API's consumption where MB is the desired unit.
func formatBytesForAPI(b uint64) float64 {
	return float64(b) / (1024 * 1024) // Convert bytes to megabytes.
}

// DashboardHandler returns an http.HandlerFunc that serves the GET /api/v1/dashboard endpoint.
// This handler collects real-time statistics and status information from the ProxyServer and KeyManager,
// transforms this data into the api.DashboardData structure, and responds with it as JSON.
//
// Dependencies:
//   - proxy *geminiproxy.ProxyServer: The main proxy server instance to fetch global stats and handler info.
//   - keyManager *geminiproxy.KeyManager: The key manager instance to fetch API key performance and status.
//
// HTTP Responses:
//   - 200 OK: Successfully retrieved and returned dashboard data. Content-Type: application/json.
//   - 500 Internal Server Error: If essential components (ProxyServer, KeyManager) are not initialized,
//     or if there's an error during JSON marshaling of the response.
func DashboardHandler(proxy *geminiproxy.ProxyServer, keyManager *geminiproxy.KeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if proxy == nil || keyManager == nil {
			log.Println("ERROR: DashboardHandler: ProxyServer or KeyManager not initialized")
			http.Error(w, "Internal server error: components not available", http.StatusInternalServerError)
			return
		}

		// Fetch raw data
		pStats := proxy.GetGlobalProxyStats()
		hInfo := proxy.GetHandlerInfo()
		keyInfos := keyManager.GetKeyInfoSnapshot()

		// --- Transform data ---

		// Handler Status
		handlerStatusItems := []HandlerStatusItem{
			{
				Name:          "Gemini API Proxy", // Main proxy handler
				Status:        ProxyStatus(hInfo.Status), // Assuming hInfo.Status matches ProxyStatus values ("Online" -> "online")
				UptimeSeconds: int64(time.Since(hInfo.Uptime).Seconds()),
			},
		}
		// Normalize status string if needed (e.g. "Online" to "online")
		// For now, assume hInfo.Status is already one of the ProxyStatus enum values.
		// If not, a switch or map would be needed here.
		switch hInfo.Status {
		case "Online":
			handlerStatusItems[0].Status = StatusOnline
		case "Offline": // Example, if proxy could be offline
			handlerStatusItems[0].Status = StatusOffline
		case "Degraded":
			handlerStatusItems[0].Status = StatusDegraded
		default:
			handlerStatusItems[0].Status = StatusUnknown
		}


		// Recent Errors
		// Transform []geminiproxy.ErrorDetail to []api.RecentErrorItem
		recentErrorItems := make([]RecentErrorItem, len(pStats.RecentErrors))
		for i, errDetail := range pStats.RecentErrors {
			// Construct a more informative message for the API if desired, or keep it simple.
			// For now, directly using errDetail.Message which should be informative enough.
			// The api.RecentErrorItem.Source can be errDetail.SourceID.
			// Other fields from ErrorDetail (Model, APIKeyID, ErrorType) can be part of the Message
			// or added as separate fields to api.RecentErrorItem if the API schema evolves.

			// Example of enriching the message for the API:
			// errMsgForAPI := fmt.Sprintf("[%s] Model: %s, Key: %s, Type: %s, Msg: %s",
			// 	errDetail.SourceID, errDetail.Model, errDetail.APIKeyID, errDetail.ErrorType, errDetail.Message)

			recentErrorItems[i] = RecentErrorItem{
				Timestamp: errDetail.Timestamp, // This is already time.Time
				Message:   fmt.Sprintf("Type: %s, Key: %s, Model: %s, Source: %s, Details: %s", errDetail.ErrorType, errDetail.APIKeyID, errDetail.Model, errDetail.SourceID, errDetail.Message),
				Source:    errDetail.SourceID,  // Or a more generic "ProxyOperation"
			}
		}

		// Proxy Statistics
		var overallSuccessRate float64
		if pStats.TotalRequests > 0 {
			overallSuccessRate = (float64(pStats.TotalSuccesses) / float64(pStats.TotalRequests)) * 100
		}

		model1Stats := ModelRequestStats{ // Assuming pStats directly maps to the primary model
			TotalRequests:      int64(pStats.TotalRequests),
			SuccessfulRequests: int64(pStats.TotalSuccesses),
			FailedRequests:     int64(pStats.TotalFailures),
			AverageLatencyMs:   float64(pStats.OverallAverageLatencyMicroseconds) / 1000.0,
			SuccessRatePercent: overallSuccessRate,
			// RequestsPerMinute and Token stats are not currently tracked by proxy.go
			RequestsPerMinute:    0,
			TokensProcessedTotal: 0,
			TokensPerMinute:      0,
		}
		proxyStatistics := ProxyStatistics{
			OverallSuccessRatePercent: overallSuccessRate,
			Model1Requests:            model1Stats,
			Model2Requests:            ModelRequestStats{}, // Zeroed out as placeholder
		}

		// API Key Performance
		apiKeyPerfItems := make([]ApiKeyPerformanceItem, len(keyInfos))
		for i, ki := range keyInfos {
			var keySuccessRate float64
			if ki.Requests > 0 {
				keySuccessRate = (float64(ki.Successes) / float64(ki.Requests)) * 100
			}
			apiKeyPerfItems[i] = ApiKeyPerformanceItem{
				KeyAlias:             ki.Name, // Using the masked name as alias
				TotalRequests:        int64(ki.Requests),
				SuccessfulRequests:   int64(ki.Successes),
				FailedRequests:       int64(ki.Failures),
				AverageLatencyMs:     float64(ki.AverageLatencyMicroseconds) / 1000.0,
				SuccessRatePercent:   keySuccessRate,
				IsEnabled:            ki.Enabled,
				// RPS and Token stats not tracked
			}
		}
		// Sort for Top/Bottom (example: by success rate, then by requests for tie-breaking)
		sort.SliceStable(apiKeyPerfItems, func(i, j int) bool {
			if apiKeyPerfItems[i].SuccessRatePercent != apiKeyPerfItems[j].SuccessRatePercent {
				return apiKeyPerfItems[i].SuccessRatePercent > apiKeyPerfItems[j].SuccessRatePercent // Higher success rate first
			}
			return apiKeyPerfItems[i].TotalRequests > apiKeyPerfItems[j].TotalRequests // More requests first for ties
		})

		topN := 5
		var topKeys []ApiKeyPerformanceItem
		if len(apiKeyPerfItems) <= topN {
			topKeys = apiKeyPerfItems
		} else {
			topKeys = apiKeyPerfItems[:topN]
		}

		// For bottom, reverse sort or sort by failure rate, etc.
		// Simple example: take the bottom N from the end of the primary sort if list is large enough,
		// or those with lowest success rate.
		sort.SliceStable(apiKeyPerfItems, func(i, j int) bool { // Sort ascending for bottom
			if apiKeyPerfItems[i].SuccessRatePercent != apiKeyPerfItems[j].SuccessRatePercent {
				return apiKeyPerfItems[i].SuccessRatePercent < apiKeyPerfItems[j].SuccessRatePercent // Lower success rate first
			}
			return apiKeyPerfItems[i].TotalRequests < apiKeyPerfItems[j].TotalRequests // Fewer requests first for ties
		})
		var bottomKeys []ApiKeyPerformanceItem
		if len(apiKeyPerfItems) <= topN {
			bottomKeys = apiKeyPerfItems // if few keys, top and bottom might be same/overlap significantly
		} else {
			bottomKeys = apiKeyPerfItems[:topN]
		}

		apiKeyPerformance := APIKeyPerformance{
			TopPerformingKeys:    topKeys,
			BottomPerformingKeys: bottomKeys,
		}


		// System Information
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		systemInformation := SystemInformation{
			CPUUsagePercent:    0, // Not implemented in proxy.go, set to 0 or specific "N/A" indicator if preferred
			MemoryUsageMB:      formatBytesForAPI(memStats.Alloc),
			TotalMemoryMB:      formatBytesForAPI(memStats.Sys),
			ProxyUptimeSeconds: int64(time.Since(hInfo.Uptime).Seconds()),
		}

		// Assemble DashboardData
		dashboardData := DashboardData{
			LastUpdated:       time.Now(),
			HandlerStatus:     handlerStatusItems,
			RecentErrors:      recentErrorItems,
			ProxyStatistics:   proxyStatistics,
			APIKeyPerformance: apiKeyPerformance,
			SystemInformation: systemInformation,
		}

		// Marshal and send response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(dashboardData); err != nil {
			log.Printf("ERROR: DashboardHandler: Failed to marshal JSON: %v", err)
			// Avoid writing to http.Error if headers/body already partially written
			// For robust error handling, check if headers sent before writing error.
			// http.Error(w, "Internal server error: failed to construct response", http.StatusInternalServerError)
			return
		}
	}
}
